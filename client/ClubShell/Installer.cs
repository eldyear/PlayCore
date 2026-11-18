using System.Collections.Generic;
using System.Diagnostics;
using System.IO;
using System.Windows;
using Microsoft.Win32;

namespace Club.Shell;

/// <summary>
/// Установка/откат киоск-режима:
/// - замена оболочки Windows на агента;
/// - отключение Диспетчера задач и прочих комбинаций;
/// - регистрация службы-сторожа.
/// </summary>
public static class Installer
{
    private const string ServiceName = "GCAgentWatchdog";

    public static void Install(string shellExe)
    {
        var serviceExe = Path.Combine(AppContext.BaseDirectory, "ClubAgent.Service.exe");
        var messages = new List<string>();

        // 1) замена оболочки
        try
        {
            Registry.SetValue(
                @"HKEY_CURRENT_USER\Software\Microsoft\Windows NT\CurrentVersion\Winlogon",
                "Shell", $"\"{shellExe}\"");
            messages.Add("[+] Оболочка заменена на агента");
        }
        catch (Exception ex) { messages.Add("[!] Ошибка замены оболочки: " + ex.Message); }

        // 2) отключить Диспетчер задач и прочее
        try
        {
            using var sys = Registry.CurrentUser.CreateSubKey(@"Software\Microsoft\Windows\CurrentVersion\Policies\System");
            sys.SetValue("DisableTaskMgr", 1, RegistryValueKind.DWord);
            sys.SetValue("DisableLockWorkstation", 1, RegistryValueKind.DWord);
            sys.SetValue("DisableChangePassword", 1, RegistryValueKind.DWord);

            using var explorer = Registry.CurrentUser.CreateSubKey(@"Software\Microsoft\Windows\CurrentVersion\Policies\Explorer");
            explorer.SetValue("NoLogoff", 1, RegistryValueKind.DWord);

            messages.Add("[+] Диспетчер задач отключён");
        }
        catch (Exception ex) { messages.Add("[!] Ошибка отключения диспетчера: " + ex.Message); }

        // 3) служба-сторож (нужны права администратора)
        try
        {
            RunSc($"create {ServiceName} binPath= \"{serviceExe}\" start= auto");
            messages.Add("[+] Служба-сторож установлена");
        }
        catch (Exception ex)
        {
            messages.Add("[!] Служба не установлена (запустите -install от администратора): " + ex.Message);
        }

        messages.Add("");
        messages.Add("Готово. Перезайдите в систему или перезагрузите ПК.");
        MessageBox.Show(string.Join("\n", messages), "PlayCore — установка", MessageBoxButton.OK, MessageBoxImage.Information);
    }

    public static void Uninstall()
    {
        var messages = new List<string>();

        try
        {
            Registry.SetValue(
                @"HKEY_CURRENT_USER\Software\Microsoft\Windows NT\CurrentVersion\Winlogon",
                "Shell", "explorer.exe");

            using var sys = Registry.CurrentUser.CreateSubKey(@"Software\Microsoft\Windows\CurrentVersion\Policies\System");
            sys.SetValue("DisableTaskMgr", 0, RegistryValueKind.DWord);
            sys.SetValue("DisableLockWorkstation", 0, RegistryValueKind.DWord);
            sys.SetValue("DisableChangePassword", 0, RegistryValueKind.DWord);

            using var explorer = Registry.CurrentUser.CreateSubKey(@"Software\Microsoft\Windows\CurrentVersion\Policies\Explorer");
            explorer.SetValue("NoLogoff", 0, RegistryValueKind.DWord);

            messages.Add("[+] Оболочка и Диспетчер задач восстановлены");
        }
        catch (Exception ex) { messages.Add("[!] Ошибка отката: " + ex.Message); }

        try
        {
            RunSc("stop " + ServiceName, throwOnError: false); // служба может быть уже остановлена
            RunSc("delete " + ServiceName);
            messages.Add("[+] Служба-сторож удалена");
        }
        catch (Exception ex)
        {
            messages.Add("[!] Служба не удалена: " + ex.Message);
        }

        messages.Add("");
        messages.Add("Готово.");
        MessageBox.Show(string.Join("\n", messages), "PlayCore — откат", MessageBoxButton.OK, MessageBoxImage.Information);
    }

    private static void RunSc(string arguments, bool throwOnError = true)
    {
        var scExe = Path.Combine(Environment.SystemDirectory, "sc.exe");
        var psi = new ProcessStartInfo(scExe, arguments)
        {
            UseShellExecute = false,
            RedirectStandardOutput = true,
            RedirectStandardError = true,
            CreateNoWindow = true
        };
        using var p = Process.Start(psi);
        var output = p.StandardOutput.ReadToEnd() + p.StandardError.ReadToEnd();
        p.WaitForExit();
        if (p.ExitCode != 0 && throwOnError)
            throw new InvalidOperationException($"sc {arguments} → код {p.ExitCode}: {output.Trim()}");
    }
}
