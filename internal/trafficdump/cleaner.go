package trafficdump

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	log "github.com/sirupsen/logrus"
)

var (
	cleanerMu     sync.Mutex
	cleanerCancel context.CancelFunc
)

// StartCleaner starts a background goroutine to periodically clean up expired or oversized traffic dump folders.
func StartCleaner(ctx context.Context, baseDir string, maxRetentionDays int, maxTotalSizeMB int, cleanIntervalHours int) {
	cleanerMu.Lock()
	defer cleanerMu.Unlock()

	StopCleaner()

	if baseDir == "" || (maxRetentionDays <= 0 && maxTotalSizeMB <= 0) {
		return
	}

	if cleanIntervalHours <= 0 {
		cleanIntervalHours = 24
	}
	interval := time.Duration(cleanIntervalHours) * time.Hour

	cleanerCtx, cancel := context.WithCancel(ctx)
	cleanerCancel = cancel

	go runCleaner(cleanerCtx, filepath.Clean(baseDir), maxRetentionDays, maxTotalSizeMB, interval)
}

// StopCleaner stops any running background cleaner.
func StopCleaner() {
	if cleanerCancel != nil {
		cleanerCancel()
		cleanerCancel = nil
	}
}

func runCleaner(ctx context.Context, baseDir string, maxRetentionDays int, maxTotalSizeMB int, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	// Initial clean
	_, _ = EnforceTrafficLimits(baseDir, maxRetentionDays, maxTotalSizeMB)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			deleted, err := EnforceTrafficLimits(baseDir, maxRetentionDays, maxTotalSizeMB)
			if err != nil {
				log.WithError(err).Warn("trafficdump: failed to enforce traffic retention limits")
			} else if deleted > 0 {
				log.Infof("trafficdump: purged %d expired/oversized traffic session folder(s)", deleted)
			}
		}
	}
}

type sessionDirInfo struct {
	path    string
	modTime time.Time
	size    int64
}

// EnforceTrafficLimits applies retention days and total size limits to traffic sessions.
func EnforceTrafficLimits(baseDir string, maxRetentionDays int, maxTotalSizeMB int) (int, error) {
	baseDir = strings.TrimSpace(baseDir)
	if baseDir == "" || (maxRetentionDays <= 0 && maxTotalSizeMB <= 0) {
		return 0, nil
	}

	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		return 0, nil
	}

	dateEntries, err := os.ReadDir(baseDir)
	if err != nil {
		return 0, err
	}

	cutoffTime := time.Time{}
	if maxRetentionDays > 0 {
		cutoffTime = time.Now().AddDate(0, 0, -maxRetentionDays)
	}

	sessions, expiredCount := collectAndPruneExpiredSessions(baseDir, dateEntries, cutoffTime, maxRetentionDays)
	oversizedCount := purgeOversizedSessions(sessions, maxTotalSizeMB)

	cleanEmptyDateDirs(baseDir)

	totalDeleted := expiredCount + oversizedCount
	return totalDeleted, nil
}

func collectAndPruneExpiredSessions(baseDir string, dateEntries []os.DirEntry, cutoffTime time.Time, maxRetentionDays int) ([]sessionDirInfo, int) {
	var sessions []sessionDirInfo
	var deletedCount int

	for _, dateEntry := range dateEntries {
		if !dateEntry.IsDir() {
			continue
		}
		dateDirPath := filepath.Join(baseDir, dateEntry.Name())
		surviving, deleted := processDateDirectory(dateDirPath, cutoffTime, maxRetentionDays)
		sessions = append(sessions, surviving...)
		deletedCount += deleted
	}

	return sessions, deletedCount
}

func processDateDirectory(dateDirPath string, cutoffTime time.Time, maxRetentionDays int) ([]sessionDirInfo, int) {
	sessionEntries, err := os.ReadDir(dateDirPath)
	if err != nil {
		return nil, 0
	}

	var surviving []sessionDirInfo
	var deletedCount int

	for _, sEntry := range sessionEntries {
		if !sEntry.IsDir() {
			continue
		}

		session, isExpired := inspectSessionDir(dateDirPath, sEntry, cutoffTime, maxRetentionDays)
		if isExpired {
			deletedCount++
			continue
		}
		if session != nil {
			surviving = append(surviving, *session)
		}
	}

	return surviving, deletedCount
}

func inspectSessionDir(dateDirPath string, sEntry os.DirEntry, cutoffTime time.Time, maxRetentionDays int) (*sessionDirInfo, bool) {
	sDirPath := filepath.Join(dateDirPath, sEntry.Name())
	info, errStat := sEntry.Info()
	if errStat != nil {
		return nil, false
	}

	modTime := info.ModTime()
	size, newestFileTime := getDirectorySizeAndModTime(sDirPath, modTime)
	if newestFileTime.After(modTime) {
		modTime = newestFileTime
	}

	isExpired := maxRetentionDays > 0 && modTime.Before(cutoffTime)
	if isExpired {
		if errRemove := os.RemoveAll(sDirPath); errRemove == nil {
			return nil, true
		}
	}

	return &sessionDirInfo{
		path:    sDirPath,
		modTime: modTime,
		size:    size,
	}, false
}

func purgeOversizedSessions(sessions []sessionDirInfo, maxTotalSizeMB int) int {
	if maxTotalSizeMB <= 0 || len(sessions) == 0 {
		return 0
	}

	maxBytes := int64(maxTotalSizeMB) * 1024 * 1024
	var totalBytes int64
	for _, s := range sessions {
		totalBytes += s.size
	}

	if totalBytes <= maxBytes {
		return 0
	}

	// Sort oldest first
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].modTime.Before(sessions[j].modTime)
	})

	var deletedCount int
	for _, s := range sessions {
		if totalBytes <= maxBytes {
			break
		}
		if errRemove := os.RemoveAll(s.path); errRemove == nil {
			deletedCount++
			totalBytes -= s.size
		}
	}

	return deletedCount
}

func getDirectorySizeAndModTime(dirPath string, defaultModTime time.Time) (int64, time.Time) {
	var totalSize int64
	newestTime := defaultModTime

	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return totalSize, newestTime
	}

	for _, entry := range entries {
		info, errInfo := entry.Info()
		if errInfo != nil {
			continue
		}
		if entry.IsDir() {
			subSize, subTime := getDirectorySizeAndModTime(filepath.Join(dirPath, entry.Name()), newestTime)
			totalSize += subSize
			if subTime.After(newestTime) {
				newestTime = subTime
			}
		} else {
			totalSize += info.Size()
			if info.ModTime().After(newestTime) {
				newestTime = info.ModTime()
			}
		}
	}

	return totalSize, newestTime
}

func cleanEmptyDateDirs(baseDir string) {
	dateEntries, err := os.ReadDir(baseDir)
	if err != nil {
		return
	}
	for _, dateEntry := range dateEntries {
		if !dateEntry.IsDir() {
			continue
		}
		dateDirPath := filepath.Join(baseDir, dateEntry.Name())
		contents, errRead := os.ReadDir(dateDirPath)
		if errRead == nil && len(contents) == 0 {
			_ = os.Remove(dateDirPath)
		}
	}
}
