; Script generated for Cadence Windows Installer (Inno Setup)
#define MyAppName "Cadence"
#define MyAppVersion "1.0.0"
#define MyAppPublisher "Adesh Deshmukh"
#define MyAppURL "https://github.com/AdeshDeshmukh/cadence"
#define MyAppExeName "cadence.exe"
#define MyAppDaemonName "cadenced.exe"

[Setup]
AppId={{5C884F70-4A92-4A73-BE8C-19453C951801}
AppName={#MyAppName}
AppVersion={#MyAppVersion}
AppPublisher={#MyAppPublisher}
AppPublisherURL={#MyAppURL}
AppSupportURL={#MyAppURL}
AppUpdatesURL={#MyAppURL}
DefaultDirName={autopf}\{#MyAppName}
DefaultGroupName={#MyAppName}
DisableProgramGroupPage=yes
LicenseFile=..\..\LICENSE
OutputBaseFilename=Cadence-Setup-{#MyAppVersion}
Compression=lzma2/ultra64
SolidCompression=yes
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=lowest
ChangesEnvironment=yes

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "envPath"; Description: "Add Cadence to PATH environment variable"; GroupDescription: "System Integration:"
Name: "startDaemon"; Description: "Automatically launch Cadence daemon at Windows startup"; GroupDescription: "System Integration:"

[Files]
Source: "..\..\dist\bin\windows\cadence.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\dist\bin\windows\cadenced.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\..\README.md"; DestDir: "{app}"; Flags: isreadme

[Registry]
Root: HKCU; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; ValueData: "{olddata};{app}"; Tasks: envPath; Check: NeedsAddPath(ExpandConstant('{app}'))
Root: HKCU; Subkey: "Software\Microsoft\Windows\CurrentVersion\Run"; ValueType: string; ValueName: "CadenceDaemon"; ValueData: """{app}\cadenced.exe"""; Tasks: startDaemon

[Icons]
Name: "{group}\{#MyAppName} CLI"; Filename: "{cmd}"; Parameters: "/k ""{app}\cadence.exe"" help"
Name: "{group}\Uninstall {#MyAppName}"; Filename: "{uninstallexe}"

[Run]
Filename: "{app}\{#MyAppDaemonName}"; Description: "Start Cadence Daemon"; Flags: nowait postinstall skipifsilent

[Code]
function NeedsAddPath(Param: string): boolean;
var
  OrigPath: string;
begin
  if not RegQueryStringValue(HKEY_CURRENT_USER, 'Environment', 'Path', OrigPath)
  then begin
    Result := True;
    exit;
  end;
  Result := Pos(';' + UpperCase(Param) + ';', ';' + UpperCase(OrigPath) + ';') = 0;
end;
