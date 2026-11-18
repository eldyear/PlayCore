using System.Diagnostics;
using System.IO;
using Club.Core;
using Microsoft.Win32;

namespace Club.Shell;

/// <summary>
/// Сбор информации о железе и дисках ПК (передаётся на сервер автоматически).
/// </summary>
public static class HardwareReporter
{
    public static HardwareReport Collect()
    {
        var info = new HardwareReport();

        // CPU — из реестра
        try
        {
            using var k = Registry.LocalMachine.OpenSubKey(@"HARDWARE\DESCRIPTION\System\CentralProcessor\0");
            info.Cpu = (k?.GetValue("ProcessorNameString") as string ?? "").Trim();
        }
        catch { }

        // RAM + GPU — через WMI (PowerShell)
        try
        {
            var ps =
                "Get-CimInstance Win32_ComputerSystem | Select-Object -ExpandProperty TotalPhysicalMemory;" +
                "Get-CimInstance Win32_VideoController | Select-Object -First 1 -ExpandProperty Name";
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
            if (p.WaitForExit(10000))
            {
                var lines = output.Split(new[] { '\r', '\n' }, StringSplitOptions.RemoveEmptyEntries);
                if (lines.Length >= 1 && long.TryParse(lines[0].Trim(), out var ramBytes))
                    info.Ram = FormatBytes(ramBytes);
                if (lines.Length >= 2)
                    info.Gpu = lines[1].Trim();
            }
        }
        catch { }

        // Диски — фиксированные разделы
        try
        {
            foreach (var drive in DriveInfo.GetDrives())
            {
                if (drive.IsReady && drive.DriveType == DriveType.Fixed)
                {
                    info.Disks.Add(new DiskInfo
                    {
                        Name = drive.Name.TrimEnd('\\'),
                        Total = drive.TotalSize,
                        Free = drive.TotalFreeSpace
                    });
                }
            }
        }
        catch { }

        return info;
    }

    private static string FormatBytes(long bytes)
    {
        var gb = bytes / 1024.0 / 1024.0 / 1024.0;
        return gb >= 100 ? ((long)Math.Round(gb)) + " ГБ" : gb.ToString("0.#") + " ГБ";
    }
}
