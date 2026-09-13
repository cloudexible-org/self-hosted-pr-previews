# Run once in an elevated PowerShell on the Windows machine (host.kind: wsl2).
#
# WSL stops a distro a few seconds after the last Windows process attached to it
# exits — systemd services inside do not count. This scheduled task holds the
# preview host's distro open from boot, with nobody logged in, and restarts it if
# it ever exits.

$distro = "preview-host"   # host.wsl_distro

# The account that owns the distro. Use this exact form: building it from
# $env:USERDOMAIN can produce a name Windows cannot map to an account.
$user = [Security.Principal.WindowsIdentity]::GetCurrent().Name

$action = New-ScheduledTaskAction -Execute "C:\Windows\System32\wsl.exe" `
  -Argument "-d $distro --exec sleep infinity"
$trigger = New-ScheduledTaskTrigger -AtStartup
# S4U: runs whether or not the user is logged on, without storing a password.
$principal = New-ScheduledTaskPrincipal -UserId $user -LogonType S4U -RunLevel Limited
$settings = New-ScheduledTaskSettingsSet `
  -ExecutionTimeLimit ([TimeSpan]::Zero) `
  -RestartCount 999 -RestartInterval (New-TimeSpan -Minutes 1) `
  -AllowStartIfOnBatteries -DontStopIfGoingOnBatteries `
  -StartWhenAvailable -MultipleInstances IgnoreNew

Register-ScheduledTask -TaskName "WSL $distro keepalive" `
  -Description "Keeps the $distro WSL distro (PR preview host) running from boot" `
  -Action $action -Trigger $trigger -Principal $principal -Settings $settings
Start-ScheduledTask -TaskName "WSL $distro keepalive"
