; Inno Setup script for myTraffic
; Build the binary first (see build.ps1), then compile this with:
;   "C:\Program Files (x86)\Inno Setup 6\ISCC.exe" installer\myTraffic.iss
; It produces installer\Output\myTraffic-Setup-<version>.exe

#define AppName "myTraffic"
#define AppVersion "1.0.0"
#define AppPublisher "qub-official"
#define AppURL "https://github.com/qub-official/myTraffic"
#define AppExe "mytraffic.exe"

[Setup]
AppId={{B6F2B1B4-9C0E-4E3A-9E7E-6A1C7D2E4F10}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
OutputDir=Output
OutputBaseFilename=myTraffic-Setup-{#AppVersion}
Compression=lzma2
SolidCompression=yes
WizardStyle=modern
ArchitecturesAllowed=x64compatible
ArchitecturesInstallIn64BitMode=x64compatible
UninstallDisplayIcon={app}\{#AppExe}
; Per-user install needs no admin rights.
PrivilegesRequired=lowest

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Tasks]
Name: "desktopicon"; Description: "Create a desktop shortcut"; GroupDescription: "Additional icons:"
Name: "addtopath"; Description: "Add mytraffic to PATH (for the CLI and MCP)"; GroupDescription: "Command line:"

[Files]
Source: "..\app\mytraffic.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "..\README.md"; DestDir: "{app}"; Flags: ignoreversion isreadme
Source: "..\LICENSE"; DestDir: "{app}"; Flags: ignoreversion

[Icons]
Name: "{group}\myTraffic"; Filename: "{app}\{#AppExe}"; Parameters: "proxy"
Name: "{group}\Uninstall myTraffic"; Filename: "{uninstallexe}"
Name: "{autodesktop}\myTraffic"; Filename: "{app}\{#AppExe}"; Parameters: "proxy"; Tasks: desktopicon

[Registry]
; Add install dir to the user's PATH when the task is selected.
Root: HKCU; Subkey: "Environment"; ValueType: expandsz; ValueName: "Path"; \
  ValueData: "{olddata};{app}"; Check: NeedsAddPath('{app}'); Tasks: addtopath

[Run]
Filename: "{app}\{#AppExe}"; Parameters: "proxy"; Description: "Launch myTraffic now"; Flags: nowait postinstall skipifsilent

[Code]
function NeedsAddPath(Param: string): Boolean;
var
  OrigPath: string;
begin
  if not RegQueryStringValue(HKCU, 'Environment', 'Path', OrigPath) then
  begin
    Result := True;
    exit;
  end;
  Result := Pos(';' + ExpandConstant(Param) + ';', ';' + OrigPath + ';') = 0;
end;
