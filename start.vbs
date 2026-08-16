Set ws = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")

currentDir = fso.GetParentFolderName(WScript.ScriptFullName)
ws.CurrentDirectory = currentDir

configPath = fso.BuildPath(currentDir, "..\..\config.yaml")
If Not fso.FileExists(configPath) Then
    configPath = fso.BuildPath(currentDir, "config.yaml")
End If
exePath = currentDir & "\cli-proxy-api.exe"

If fso.FileExists(exePath) Then
    cmd = """" & exePath & """ --config """ & configPath & """ --port 18318"
Else
    goExe = "go"
    If fso.FileExists("C:\Program Files\Go\bin\go.exe") Then
        goExe = """C:\Program Files\Go\bin\go.exe"""
    End If
    cmd = "cmd.exe /c " & goExe & " run ./cmd/server --config """ & configPath & """ --port 18318"
End If

ws.Run cmd, 0, False

Set ws = Nothing
Set fso = Nothing
