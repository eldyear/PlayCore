using System.Text.RegularExpressions;

namespace Club.Core;

/// <summary>
/// Автоопределение установленных в Steam игр:
/// читает libraryfolders.vdf (список библиотек) и appmanifest_*.acf (названия игр),
/// без вмешательства в процессы Steam. Только чтение конфигурационных файлов.
/// </summary>
public class SteamDetector
{
    private readonly string _steamRoot;

    /// <summary>Служебные/неигровые приложения Steam, которые не показываем как игры.</summary>
    private static readonly HashSet<string> NonGameApps = new(StringComparer.OrdinalIgnoreCase)
    {
        "Steamworks Common Redistributables",
        "Proton",
        "Proton Experimental",
        "Proton Hotfix",
        "Proton 5.0",
        "Proton 6.3",
        "Proton 7.0",
        "Proton 8.0",
        "Proton 9.0",
        "Steam Linux Runtime",
        "Steam Linux Runtime - Soldier",
        "Steam Linux Runtime - Sniper",
        "Steam Linux Runtime 1.0 (scout)",
        "Steam Linux Runtime 2.0 (soldier)",
        "Steam Linux Runtime 3.0 (sniper)",
        "SteamVR",
        "Steam 360 Video Player",
        "Steamworks SDK Redist",
        "Source SDK Base 2007",
        "Source SDK Base 2013",
    };

    public SteamDetector(string steamRoot)
    {
        _steamRoot = steamRoot;
    }

    public static string DefaultSteamRoot()
    {
        return Path.Combine(
            Environment.GetFolderPath(Environment.SpecialFolder.ProgramFilesX86),
            "Steam");
    }

    /// <summary>Возвращает список установленных игр как команды steam://rungameid/…</summary>
    public List<GameConfig> Detect()
    {
        var result = new List<GameConfig>();
        try
        {
            if (string.IsNullOrEmpty(_steamRoot) || !Directory.Exists(_steamRoot))
                return result;

            var libraries = new List<string> { _steamRoot };
            libraries.AddRange(ParseLibraryFolders(Path.Combine(_steamRoot, "steamapps", "libraryfolders.vdf")));

            var seen = new HashSet<long>();
            foreach (var lib in libraries.Distinct())
            {
                var steamapps = Path.Combine(lib, "steamapps");
                if (!Directory.Exists(steamapps)) continue;

                foreach (var acf in Directory.GetFiles(steamapps, "appmanifest_*.acf"))
                {
                    var (appId, name) = ParseManifest(acf);
                    if (appId > 0 && !string.IsNullOrWhiteSpace(name) && seen.Add(appId) && !NonGameApps.Contains(name))
                    {
                        result.Add(new GameConfig
                        {
                            Name = name,
                            Command = $"steam://rungameid/{appId}",
                            AppId = appId
                        });
                    }
                }
            }
        }
        catch
        {
            // любая ошибка чтения Steam-файлов не должна ронять агент
        }

        try { return result.OrderBy(g => g.Name, StringComparer.OrdinalIgnoreCase).ToList(); }
        catch { return result; }
    }

    private static IEnumerable<string> ParseLibraryFolders(string path)
    {
        var result = new List<string>();
        try
        {
            if (!File.Exists(path)) return result;
            var text = File.ReadAllText(path);
            foreach (Match m in Regex.Matches(text, "\"path\"\\s*\"(.*?)\""))
            {
                var p = m.Groups[1].Value.Replace("\\\\", "\\");
                if (!string.IsNullOrWhiteSpace(p) && !result.Contains(p))
                    result.Add(p);
            }
        }
        catch { }
        return result;
    }

    private static (long appId, string name) ParseManifest(string path)
    {
        try
        {
            var text = File.ReadAllText(path);
            long appId = 0;
            var idMatch = Regex.Match(text, "\"appid\"\\s*\"(\\d+)\"");
            if (idMatch.Success) long.TryParse(idMatch.Groups[1].Value, out appId);

            string name = "";
            var nameMatch = Regex.Match(text, "\"name\"\\s*\"(.*?)\"");
            if (nameMatch.Success) name = nameMatch.Groups[1].Value;

            return (appId, name);
        }
        catch
        {
            return (0, "");
        }
    }
}
