using System.Diagnostics;
using System.Text.Json;
using Club.Core;
using Microsoft.Win32;

namespace Club.Shell;

/// <summary>
/// Проверка готовности ПК к FACEIT (только чтение, ничего не меняет в системе).
/// Собирает Windows / Secure Boot / Test signing из реестра и TPM / VBS через WMI (PowerShell).
/// </summary>
public static class ComplianceChecker
{
    public static ComplianceReport Collect()
    {
        var r = new ComplianceReport
        {
            CheckedAt = DateTimeOffset.UtcNow.ToUnixTimeSeconds()
        };

        (r.Windows, r.WindowsOk) = WindowsVersion();
        (r.SecureBoot, r.SecureBootOk) = SecureBootState();
        (r.TestSigning, r.TestSigningOk) = TestSigningState();
        (r.Tpm, r.TpmOk, r.Vbs, r.VbsOk) = TpmVbs();

        r.Verdict = ComputeVerdict(r);
        return r;
    }

    private static string ComputeVerdict(ComplianceReport r)
    {
        // жёсткие требования FACEIT
        if (!r.WindowsOk || !r.SecureBootOk || !r.TpmOk || !r.TestSigningOk)
            return "fail";
        // предупреждение: VBS выключен или что-то не прочиталось
        if (!r.VbsOk || (r.Windows + r.Tpm + r.SecureBoot + r.Vbs + r.TestSigning).Contains("unknown"))
            return "warn";
        return "ok";
    }

    private static (string, bool) WindowsVersion()
    {
        try
        {
            using var k = Registry.LocalMachine.OpenSubKey(@"SOFTWARE\Microsoft\Windows NT\CurrentVersion");
            if (k == null) return ("unknown", false);
            var product = k.GetValue("ProductName") as string ?? "";
            var build = k.GetValue("CurrentBuildNumber") as string ?? "";
            var display = k.GetValue("DisplayVersion") as string ?? "";

            int.TryParse(build, out var buildNum);

            // ProductName на Windows 11 иногда ошибочно пишет "Windows 10",
            // поэтому версию определяем строго по номеру сборки (>= 22000 = 11).
            var baseName = buildNum >= 22000 ? "Windows 11"
                         : buildNum >= 10240 ? "Windows 10" : "Windows";

            var edition = product;
            edition = edition.Replace("Windows 11", "").Replace("Windows 10", "").Trim();

            var name = baseName;
            if (edition.Length > 0 && edition != "Windows")
                name += " " + edition;
            if (display.Length > 0)
                name += " " + display;
            if (build.Length > 0)
                name += " (build " + build + ")";

            return (name, buildNum >= 22000);
        }
        catch
        {
            return ("unknown", false);
        }
    }

    private static (string, bool) SecureBootState()
    {
        try
        {
            using var k = Registry.LocalMachine.OpenSubKey(@"SYSTEM\CurrentControlSet\Control\SecureBoot\State");
            if (k == null) return ("unknown", false);
            var v = k.GetValue("UEFISecureBootEnabled");
            if (v is int i && i == 1) return ("on", true);
            return ("off", false);
        }
        catch
        {
            return ("unknown", false);
        }
    }

    private static (string, bool) TestSigningState()
    {
        try
        {
            using var k = Registry.LocalMachine.OpenSubKey(@"SYSTEM\CurrentControlSet\Control");
            if (k == null) return ("unknown", false);
            var opts = k.GetValue("SystemStartOptions") as string ?? "";
            if (opts.ToUpperInvariant().Contains("TESTSIGNING")) return ("on", false);
            return ("off", true);
        }
        catch
        {
            return ("unknown", false);
        }
    }

    private static (string tpm, bool tpmOk, string vbs, bool vbsOk) TpmVbs()
    {
        string tpm = "unknown", vbs = "unknown";
        bool tpmOk = false, vbsOk = false;

        try
        {
            var ps =
                "$t = Get-CimInstance -Namespace 'root/CIMv2/Security/MicrosoftTpm' -ClassName Win32_Tpm -ErrorAction SilentlyContinue;" +
                "$d = Get-CimInstance -Namespace 'root/Microsoft/Windows/DeviceGuard' -ClassName Win32_DeviceGuard -ErrorAction SilentlyContinue;" +
                "[PSCustomObject]@{ tpm = $t.SpecVersion; tpm_enabled = $t.IsEnabled_InitialValue; vbs = $d.VirtualizationBasedSecurityStatus } | ConvertTo-Json -Compress";

            using var p = new Process
            {
                StartInfo = new ProcessStartInfo("powershell", "-NoProfile -NonInteractive -Command \"" + ps + "\"")
                {
                    UseShellExecute = false,
                    RedirectStandardOutput = true,
                    RedirectStandardError = true,
                    CreateNoWindow = true
                }
            };
            p.Start();
            var output = p.StandardOutput.ReadToEnd();
            if (!p.WaitForExit(15000))
            {
                try { p.Kill(); } catch { }
                return (tpm, tpmOk, vbs, vbsOk);
            }

            using var doc = JsonDocument.Parse(output);
            var root = doc.RootElement;

            if (root.TryGetProperty("tpm", out var tpmEl) && tpmEl.ValueKind == JsonValueKind.String)
            {
                tpm = tpmEl.GetString() ?? "unknown";
                var enabled = root.TryGetProperty("tpm_enabled", out var enEl) && enEl.ValueKind == JsonValueKind.True;
                tpmOk = tpm.Contains("2.0") && enabled;
            }

            if (root.TryGetProperty("vbs", out var vbsEl) && vbsEl.TryGetInt32(out var vbsVal))
            {
                if (vbsVal == 2) { vbs = "running"; vbsOk = true; }
                else if (vbsVal == 1) { vbs = "enabled (not running)"; vbsOk = false; }
                else { vbs = "off"; vbsOk = false; }
            }
        }
        catch
        {
            // оставляем unknown
        }

        return (tpm, tpmOk, vbs, vbsOk);
    }
}
