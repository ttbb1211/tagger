; Tagger Windows 安装包（Inno Setup 6）
; 构建方式：ISCC /DAPP_VERSION=x.y.z tagger.iss
; 前置：dist\tagger.exe 已由 CI 构建完成
; 注意：ISCC 的相对路径以本 .iss 文件所在目录为基准，仓库根 = ..\..

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
; ★ 必须显式写 no。DisableDirPage 的默认值是 auto —— 它会在启动时查注册表，
;   发现同一 AppId 已安装就「不显示选择安装位置页」，并静默沿用上次的目录
;   （配合 UsePreviousAppDir 的默认 yes）。症状：升级时向导直接从许可页跳到
;   自定义的「选择音乐库目录」页，用户看不到安装位置、也不知道装到哪去了。
;   写 no 后该页始终显示，且默认值仍会被 UsePreviousAppDir 预填为上次的目录。
DisableDirPage=no
; 「准备安装」页显示目标目录，安装前可再确认一次装到哪里
AlwaysShowDirOnReadyPage=yes
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
SetupIconFile=../../frontend/public/favicon.ico
LicenseFile=../../LICENSE
OutputDir=../../release
OutputBaseFilename=tagger_setup_{#APP_VERSION}_windows_amd64
Compression=lzma2/max
SolidCompression=yes
ArchitecturesInstallIn64BitMode=x64compatible
PrivilegesRequired=admin
UninstallDisplayIcon={app}\tagger.exe

[Files]
Source: "..\..\dist\tagger.exe"; DestDir: "{app}"; Flags: ignoreversion
Source: "如何设置音乐目录.txt"; DestDir: "{app}"; Check: IsSkipMusicDir

[Dirs]
; 数据目录放在用户区，避开 Program Files 的写权限限制
Name: "{localappdata}\{#DataDirName}"

[Icons]
Name: "{group}\{#AppName}"; Filename: "{app}\tagger.exe"; Parameters: "{code:ShortcutParams}"; WorkingDir: "{localappdata}\{#DataDirName}"; Comment: "启动 Tagger（本机 127.0.0.1:8080）"
Name: "{group}\{#AppName} 网页界面"; Filename: "http://127.0.0.1:8080"
Name: "{group}\卸载 {#AppName}"; Filename: "{uninstallexe}"
Name: "{commondesktop}\{#AppName}"; Filename: "{app}\tagger.exe"; Parameters: "{code:ShortcutParams}"; WorkingDir: "{localappdata}\{#DataDirName}"; Tasks: desktopicon

[Tasks]
Name: "desktopicon"; Description: "{cm:CreateDesktopIcon}"; GroupDescription: "{cm:AdditionalIcons}"; Flags: unchecked

[Run]
Filename: "{app}\tagger.exe"; Parameters: "{code:ShortcutParams}"; WorkingDir: "{localappdata}\{#DataDirName}"; Description: "启动 {#AppName}"; Flags: nowait postinstall skipifsilent

[UninstallDelete]
; 卸载不删音乐库与数据目录；两个桌面位置的快捷方式都清扫，避免残留
Type: files; Name: "{app}\tagger.exe"
Type: files; Name: "{userdesktop}\Tagger.lnk"
Type: files; Name: "{commondesktop}\Tagger.lnk"

[Code]
var
  MusicDirPage: TInputDirWizardPage;
  SkipCheckbox: TNewCheckBox;
  DeleteData: Boolean;
  PrevInstallDir: string;

{ 覆盖安装前先卸载旧版（v1.6.6 新增）。
  ⚠️ Inno Setup 对「同一 AppId 已装旧版」的默认行为只是覆盖文件，
  不会先卸载、也不弹任何提示（v1.6.5 覆盖安装实测：无「检测到旧版」询问，
  旧版卸载器也未运行）。这里在 InitializeSetup 显式查注册表卸载项：
  - 发现旧版 → 弹窗询问，选「是」用 /SILENT 运行旧版卸载器并等待结束。
    旧版 v1.6.2+ 的卸载器会先弹「是否保留 TaggerData」（默认保留），
    用户对数据去留仍有决定权；不传 /SUPPRESSMSGBOXES 就是为了保住这条提示。
  - 选「否」→ 跳过卸载继续覆盖安装（文件 ignoreversion 覆盖，仍可升级成功）。
  - 先记下旧 InstallLocation，卸载会删掉注册表项、UsePreviousAppDir 就没有
    预填来源了，故在 InitializeWizard 里手动回填到安装位置页（保住 v1.6.5
    起的「预填上次目录」体验）。}
function InitializeSetup(): Boolean;
var
  KeyPath, UninstallString, DisplayVersion: string;
  ResultCode: Integer;
begin
  Result := True;
  { ⚠️ Pascal 字符串不做 Inno 常量转义，GUID 花括号直接写单层即可；
    双花括号转义是条目值（如 AppId）那套展开规则，别混用 }
  KeyPath := 'SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\{7E4A2C19-8B3D-4F6A-9C21-D5E8F0A63B47}_is1';
  if RegQueryStringValue(HKLM, KeyPath, 'UninstallString', UninstallString) then
  begin
    RegQueryStringValue(HKLM, KeyPath, 'InstallLocation', PrevInstallDir);
    if not RegQueryStringValue(HKLM, KeyPath, 'DisplayVersion', DisplayVersion) then
      DisplayVersion := '未知版本';
    if MsgBox(
        '检测到本机已安装 Tagger ' + DisplayVersion + '。' + #13#10 + #13#10 +
        '建议先卸载旧版再继续安装新版。' + #13#10 +
        '曲库数据 TaggerData 默认保留，卸载旧版时会再次询问。' + #13#10 + #13#10 +
        '要现在卸载旧版吗？',
        mbConfirmation, MB_YESNO) = IDYES then
      Exec(RemoveQuotes(UninstallString), '/SILENT /NORESTART', '',
        SW_SHOW, ewWaitUntilTerminated, ResultCode);
  end;
end;

procedure InitializeWizard;
begin
  { 旧版卸载后注册表已被清掉，这里手动回填上次安装目录（见 InitializeSetup 注释） }
  if PrevInstallDir <> '' then
    WizardForm.DirEdit.Text := PrevInstallDir;
  MusicDirPage := CreateInputDirPage(wpSelectDir,
    '选择音乐库目录', 'Tagger 将扫描并管理该目录下的音乐文件（会直接修改文件内嵌标签，请确保有备份）',
    '选择音乐库根目录，然后点击「下一步」。',
    False, '');
  MusicDirPage.Add('');
  MusicDirPage.Values[0] := GetEnv('USERPROFILE') + '\Music';

  SkipCheckbox := TNewCheckBox.Create(WizardForm);
  SkipCheckbox.Parent := MusicDirPage.Surface;
  SkipCheckbox.Left := MusicDirPage.Edits[0].Left;
  SkipCheckbox.Top := MusicDirPage.Edits[0].Top + MusicDirPage.Edits[0].Height + ScaleY(16);
  SkipCheckbox.Width := MusicDirPage.Surface.Width - SkipCheckbox.Left;
  SkipCheckbox.Height := ScaleY(20);
  SkipCheckbox.Caption := '暂不设置，安装完成后手动指定音乐目录';
end;

function IsSkipMusicDir: Boolean;
begin
  Result := (SkipCheckbox <> nil) and SkipCheckbox.Checked;
end;

{ 快捷方式与 [Run] 的启动参数：勾选跳过时不带 -music-dir }
function ShortcutParams(Param: string): string;
begin
  if IsSkipMusicDir then
    Result := '-listen 127.0.0.1:8080 -data-dir "' + ExpandConstant('{localappdata}') + '\' + '{#DataDirName}"'
  else
    Result := '-listen 127.0.0.1:8080 -data-dir "' + ExpandConstant('{localappdata}') + '\' + '{#DataDirName}" -music-dir "' + MusicDirPage.Values[0] + '"';
end;

function GetMusicDir(Param: string): string;
begin
  Result := MusicDirPage.Values[0];
end;

function NextButtonClick(CurPageID: Integer): Boolean;
begin
  Result := True;
  if (CurPageID = MusicDirPage.ID) and (not IsSkipMusicDir) then
  begin
    if Trim(MusicDirPage.Values[0]) = '' then
    begin
      MsgBox('请选择音乐库目录，或勾选「暂不设置」。', mbError, MB_OK);
      Result := False;
    end;
  end;
end;

procedure CurStepChanged(CurStep: TSetupStep);
begin
  if (CurStep = ssPostInstall) and IsSkipMusicDir then
    MsgBox('您选择了稍后设置音乐目录。' #13#10 #13#10 '设置方法：右键开始菜单中的「Tagger」快捷方式 → 属性 → 在「目标」末尾加上  -music-dir "你的音乐目录" 。' #13#10 '详细说明见安装目录下的《如何设置音乐目录.txt》。', mbInformation, MB_OK);
end;

{ 卸载时询问是否一并删除用户数据（曲库索引/设置/缓存）。
  ⚠️ Inno Setup 的 CreateInputOptionPage 只能在安装阶段调用，
  卸载阶段调用会报 "Cannot call CreateInputOptionPage function during Uninstall."
  （v1.5.1 的回归老板安装 v1.5.1 后卸不掉的根因）。
  卸载时拿用户输入的标准做法 = MsgBox（或 TaskDialog）：
  这里用 MsgBox 的 Yes/No，默认 Yes = 保留数据（与老板「默认不勾」意图一致），
  只有用户主动选 No 才删除 TaggerData（不可恢复）。}
procedure CurUninstallStepChanged(CurStep: TUninstallStep);
begin
  if CurStep = usUninstall then
    DeleteData := MsgBox(
      '卸载时是否保留 Tagger 的用户数据？' + #13#10 + #13#10 +
      '位置：' + ExpandConstant('{localappdata}') + '\TaggerData' + #13#10 +
      '包含：曲库索引、设置、缓存' + #13#10 + #13#10 +
      '选「是」= 保留数据（默认，重装后可继续用）' + #13#10 +
      '选「否」= 一并删除（不可恢复）',
      mbConfirmation, MB_YESNO) = IDNO;
  if (CurStep = usPostUninstall) and DeleteData then
    DelTree(ExpandConstant('{localappdata}\TaggerData'), False, True, True);
end;
