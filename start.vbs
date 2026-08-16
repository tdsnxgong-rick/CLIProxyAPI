Set ws = CreateObject("WScript.Shell")
Set fso = CreateObject("Scripting.FileSystemObject")

currentDir = fso.GetParentFolderName(WScript.ScriptFullName)
ws.CurrentDirectory = currentDir

batPath = fso.BuildPath(currentDir, "start.bat")
ws.Run "cmd.exe /c """ & batPath & """", 0, False

Set ws = Nothing
Set fso = Nothing
