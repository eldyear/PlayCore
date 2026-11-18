using Club.Core;
using Xunit;

namespace Club.Core.Tests;

public class SteamDetectorTests
{
    private static string MakeSteamTree(string root)
    {
        Directory.CreateDirectory(Path.Combine(root, "steamapps"));
        Directory.CreateDirectory(Path.Combine(root, "steamapps", "common"));
        Directory.CreateDirectory(Path.Combine(root, "..", "D", "SteamLibrary", "steamapps"));

        File.WriteAllText(Path.Combine(root, "steamapps", "libraryfolders.vdf"), """
        "libraryfolders"
        {
            "0"
            {
                "path"		"$ROOT$"
                "apps" { "730" "1" }
            }
            "1"
            {
                "path"		"$LIB$"
                "apps" { "570" "1" }
            }
        }
        """.Replace("$ROOT$", root.Replace("\\", "\\\\")).Replace("$LIB$", Path.Combine(root, "..", "D", "SteamLibrary").Replace("\\", "\\\\")));

        File.WriteAllText(Path.Combine(root, "steamapps", "appmanifest_730.acf"), """
        "AppState"
        {
            "appid"		"730"
            "name"		"Counter-Strike 2"
        }
        """);

        File.WriteAllText(Path.Combine(root, "..", "D", "SteamLibrary", "steamapps", "appmanifest_570.acf"), """
        "AppState"
        {
            "appid"		"570"
            "name"		"Dota 2"
        }
        """);

        return root;
    }

    [Fact]
    public void Detect_FindsGamesAcrossLibraries()
    {
        var root = Path.Combine(Path.GetTempPath(), "steamtest_" + Guid.NewGuid().ToString("N"));
        try
        {
            MakeSteamTree(root);
            var detector = new SteamDetector(root);
            var games = detector.Detect();

            Assert.Contains(games, g => g.Name == "Counter-Strike 2" && g.Command == "steam://rungameid/730");
            Assert.Contains(games, g => g.Name == "Dota 2" && g.Command == "steam://rungameid/570");
            Assert.Equal(2, games.Count);
        }
        finally
        {
            if (Directory.Exists(root)) Directory.Delete(root, true);
            var lib = Path.Combine(root, "..", "D");
            var libRoot = Path.Combine(root, "..", "D", "SteamLibrary");
            if (Directory.Exists(libRoot)) Directory.Delete(libRoot, true);
            if (Directory.Exists(lib)) Directory.Delete(lib, true);
        }
    }

    [Fact]
    public void Detect_NoSteam_ReturnsEmpty()
    {
        var detector = new SteamDetector(Path.Combine(Path.GetTempPath(), "does_not_exist_steam"));
        Assert.Empty(detector.Detect());
    }
}
