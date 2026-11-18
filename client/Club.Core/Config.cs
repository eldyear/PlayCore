using System.Text.Json;
using System.Text.Json.Serialization;

namespace Club.Core;

public class GameConfig
{
    [JsonPropertyName("name")] public string Name { get; set; } = "";
    [JsonPropertyName("command")] public string Command { get; set; } = "";
    [JsonPropertyName("args")] public string Args { get; set; } = "";
    [JsonPropertyName("app_id")] public long AppId { get; set; } // Steam appid (0 = нет)
}

public class AgentConfig
{
    [JsonPropertyName("server_url")] public string ServerUrl { get; set; } = "";
    [JsonPropertyName("pc_id")] public long PcId { get; set; }
    [JsonPropertyName("pc_token")] public string PcToken { get; set; } = "";
    [JsonPropertyName("poll_seconds")] public int PollSeconds { get; set; } = 2;
    [JsonPropertyName("low_threshold_minutes")] public long LowThresholdMinutes { get; set; } = 5;
    [JsonPropertyName("background_image")] public string BackgroundImage { get; set; } = "";
    [JsonPropertyName("games_folder")] public string GamesFolder { get; set; } = "";
    [JsonPropertyName("games")] public List<GameConfig> Games { get; set; } = new();
    [JsonPropertyName("exclude_games")] public List<string> ExcludeGames { get; set; } = new();
    [JsonPropertyName("nvidia_command")] public string NvidiaCommand { get; set; } = "";
    [JsonPropertyName("local_admin_password")] public string LocalAdminPassword { get; set; } = "";
    [JsonPropertyName("log_path")] public string LogPath { get; set; } = "";

    public static AgentConfig Load(string path)
    {
        if (!File.Exists(path)) return new AgentConfig();
        try
        {
            var json = File.ReadAllText(path);
            var cfg = JsonSerializer.Deserialize<AgentConfig>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true });
            return cfg ?? new AgentConfig();
        }
        catch
        {
            return new AgentConfig();
        }
    }
}
