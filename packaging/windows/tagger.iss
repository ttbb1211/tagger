; Tagger Windows 安装包（Inno Setup 6）
; 构建方式：ISCC /DAPP_VERSION=x.y.z tagger.iss
; 前置：dist\tagger.exe 已由 CI 构建完成

#define AppName "Tagger"
#define AppPublisher "ttbb1211/tagger (fork of Ericwyn/tagger)"
#define DataDirName "TaggerData"

#ifndef APP_VERSION
#define APP_VERSION "0.0.0"
#endif

[Setup]
AppId={{7E4A2C19-8B3D-4F6A-9C21-D5E8F0A63B47}
AppName={#AppName}
AppVersion={#APP_VERSION}
AppPublisher={#AppPublisher}
AppPublisherURL=https://github.com/ttbb1211/tagger
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
LicenseFile=..\..\LICENSE
OutputDir=release
OutputBaseFilename=tagger_setup_{#APP_VERSION}_windows_amd64
Compression=lzma2/max
SolidCompression=yes
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
UninstallDisplayIcon={app}\tagger.exe

[Files]
Source: "dist\tagger.exe"; DestDir: "{app}"; Flags: ignoreversion

[Dirs]
; 数据目录放在用户区，避开 Program Files 的写权限限制
Name: "{localappdata}\{#DataDirName}"

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\tagger.exe"; Parameters: "-listen 127.0.0.1:8080 -data-dir ""{localappdata}\{#DataDirName}"" -music-dir ""{code:GetMusicDir}"""; WorkingDir: "{localappdata}\{#DataDirName}"; Comment: "启动 Tagger（本机 127.0.0.1:8080）"
Name: "{group}\{#AppName} 网页界面"; Filename: "http://127.0.0.1:8080"
Name: "{group}\卸载 {#AppName}"; Filename: "{uninstallexe}"
Name: "{commondesktop}\{#AppName}"; Filename: "{app}\tagger.exe"; Parameters: "-listen 127.0.0.1:8080 -data-dir ""{localappdata}\{#DataDirName}"" -music-dir ""{code:GetMusicDir}"""; WorkingDir: "{localappdata}\{#DataDirName}"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Run]
Filename: "{app}\tagger.exe"; Parameters: "-listen 127.0.0.1:8080 -data-dir ""{localappdata}\{#DataDirName}"" -music-dir ""{code:GetMusicDir}"""; WorkingDir: "{localappdata}\{#DataDirName}"; Description: "启动 {#AppName}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; 卸载不删音乐库与数据目录，仅提示位置
Type: files; Name: "{app}\tagger.exe"

[Code]
var
  MusicDirPage: TInputDirWizardPage;

procedure InitializeWizard;
begin
  MusicDirPage := CreateInputDirPage(wpSelectDir,
    '选择音乐库目录', 'Tagger 将扫描并管理该目录下的音乐文件（会直接修改文件内嵌标签，请确保有备份）',
    '选择音乐库根目录，然后点击「下一步」。',
    False, '');
  MusicDirPage.Add('');
  MusicDirPage.Values[0] := GetEnv('USERPROFILE') + '\Music';
end;

function GetMusicDir(Param: string): string;
begin
  Result := MusicDirPage.Values[0];
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if CurPageID = MusicDirPage.ID then
  begin
    if Trim(MusicDirPage.Values[0]) = '' then
    begin
      MsgBox('请选择音乐库目录。', mbError, MB_OK);
      Result := False;
    end;
  end;
end;
