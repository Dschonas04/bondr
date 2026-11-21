@echo off
rem Baut Bondr.exe mit dem C#-Compiler, der in Windows enthalten ist (.NET Framework 4.8).
setlocal
set CSC=%WINDIR%\Microsoft.NET\Framework64\v4.0.30319\csc.exe
if not exist "%CSC%" set CSC=%WINDIR%\Microsoft.NET\Framework\v4.0.30319\csc.exe
if not exist dist mkdir dist
"%CSC%" /nologo /target:winexe /optimize+ /platform:anycpu /langversion:5 /codepage:65001 ^
  /out:dist\Bondr.exe /win32icon:res\bondr.ico /win32manifest:res\bondr.manifest ^
  /r:System.dll /r:System.Core.dll /r:System.Drawing.dll /r:System.Windows.Forms.dll /r:System.Web.Extensions.dll ^
  src\*.cs
exit /b %ERRORLEVEL%
