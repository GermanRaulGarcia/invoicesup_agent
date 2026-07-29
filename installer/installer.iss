; Inno Setup script for the InvoicesUp Connector Agent.
; Build the binary first (../build.sh), then compile this on Windows with
; Inno Setup 6+ (https://jrsoftware.org/isinfo.php) to produce the installer.

#define AppName "InvoicesUp Connector Agent"
#define AppVersion "0.1.0"
#define ExeName "invoicesup-agent.exe"

[Setup]
AppName={#AppName}
AppVersion={#AppVersion}
DefaultDirName={autopf}\InvoicesUp Agent
DisableProgramGroupPage=yes
PrivilegesRequired=admin
OutputBaseFilename=invoicesup-agent-setup
Compression=lzma2
SolidCompression=yes
ArchitecturesInstallIn64BitMode=x64
WizardStyle=modern

[Files]
Source: "..\dist\{#ExeName}"; DestDir: "{app}"; Flags: ignoreversion

[Code]
var
  ConfigPage: TInputQueryWizardPage;

procedure InitializeWizard;
begin
  ConfigPage := CreateInputQueryPage(wpSelectDir,
    'Configuración del agente',
    'Datos de conexión con InvoicesUp',
    'Complete los datos que le pasó el administrador de InvoicesUp.');
  ConfigPage.Add('URL de InvoicesUp:', False);
  ConfigPage.Add('Token de conector:', True);   { masked }
  ConfigPage.Add('Carpeta local (donde Golden importa):', False);
  ConfigPage.Values[0] := 'https://invoicesup.kordino.com';
  ConfigPage.Values[2] := ExpandConstant('{commonappdata}\InvoicesUp\exports');
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if CurPageID = ConfigPage.ID then
  begin
    if (Trim(ConfigPage.Values[0]) = '') or (Trim(ConfigPage.Values[1]) = '') or (Trim(ConfigPage.Values[2]) = '') then
    begin
      MsgBox('Complete la URL, el token y la carpeta.', mbError, MB_OK);
      Result := False;
    end;
  end;
end;

function JsonEscape(const S: String): String;
begin
  Result := S;
  StringChangeEx(Result, '\', '\\', True);
  StringChangeEx(Result, '"', '\"', True);
end;

procedure WriteConfig;
var
  Dir, Json: String;
begin
  Dir := ExpandConstant('{commonappdata}\InvoicesUp');
  ForceDirectories(Dir);
  Json :=
    '{' + #13#10 +
    '  "base_url": "' + JsonEscape(Trim(ConfigPage.Values[0])) + '",' + #13#10 +
    '  "token": "' + JsonEscape(Trim(ConfigPage.Values[1])) + '",' + #13#10 +
    '  "folder": "' + JsonEscape(Trim(ConfigPage.Values[2])) + '",' + #13#10 +
    '  "poll_seconds": 30' + #13#10 +
    '}' + #13#10;
  SaveStringToFile(Dir + '\config.json', Json, False);
end;

procedure CurStepChanged(CurStep: TSetupStep);
var
  ResultCode: Integer;
begin
  if CurStep = ssPostInstall then
  begin
    WriteConfig;
    { The exe defaults to ProgramData\InvoicesUp\config.json, so `install`
      registers the service against exactly the file we just wrote. }
    Exec(ExpandConstant('{app}\{#ExeName}'), 'install', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  end;
end;

procedure CurUninstallStepChanged(CurUninstallStep: TUninstallStep);
var
  ResultCode: Integer;
begin
  if CurUninstallStep = usUninstall then
  begin
    Exec(ExpandConstant('{app}\{#ExeName}'), 'stop', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
    Exec(ExpandConstant('{app}\{#ExeName}'), 'uninstall', '', SW_HIDE, ewWaitUntilTerminated, ResultCode);
  end;
end;
