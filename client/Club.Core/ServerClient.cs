using System.Net.Http;
using System.Text;
using System.Text.Json;

namespace Club.Core;

/// <summary>
/// HTTP-клиент сервера: опрос статуса, подтверждение команд, отчёт FACEIT, проверка пароля.
/// Переносимая логика — не зависит от UI и Windows.
/// </summary>
public class ServerClient
{
    private readonly AgentConfig _cfg;
    private readonly HttpClient _http;

    public ServerClient(AgentConfig cfg, HttpClient http = null)
    {
        _cfg = cfg;
        _http = http ?? new HttpClient { Timeout = TimeSpan.FromSeconds(5) };
    }

    private string Base => _cfg.ServerUrl.TrimEnd('/');

    public async Task<AgentStatus> GetStatusAsync()
    {
        using var req = new HttpRequestMessage(HttpMethod.Get, $"{Base}/agent/status/{_cfg.PcId}");
        req.Headers.Add("X-PC-Token", _cfg.PcToken);
        using var resp = await _http.SendAsync(req);
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        return JsonSerializer.Deserialize<AgentStatus>(json, new JsonSerializerOptions { PropertyNameCaseInsensitive = true }) ?? new AgentStatus();
    }

    public async Task AckCommandAsync(long commandId, string status)
    {
        await PostJsonAsync("/api/agent/command-ack", new { id = commandId, status });
    }

    public async Task AckMessageAsync(long messageId)
    {
        await PostJsonAsync("/api/agent/message-ack", new { id = messageId });
    }

    public async Task PostComplianceAsync(ComplianceReport report)
    {
        await PostJsonAsync("/api/agent/compliance", new { pc_id = _cfg.PcId, report });
    }

    public async Task PostHardwareAsync(HardwareReport report)
    {
        await PostJsonAsync("/api/agent/hardware", new { pc_id = _cfg.PcId, cpu = report.Cpu, ram = report.Ram, gpu = report.Gpu, disks = report.Disks });
    }

    public async Task<ClientLoginResult> ClientLoginAsync(string login, string password)
    {
        var resp = await PostJsonRawAsync("/api/agent/client-login", new { pc_id = _cfg.PcId, login, password });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        var obj = JsonSerializer.Deserialize<Dictionary<string, JsonElement>>(json);
        var result = new ClientLoginResult();
        if (obj != null)
        {
            if (obj.TryGetValue("client", out var clientEl))
                result.Client = clientEl.Deserialize<ClientInfo>() ?? new ClientInfo();
            if (obj.TryGetValue("session_started", out var startedEl))
                result.SessionStarted = startedEl.GetBoolean();
            if (obj.TryGetValue("message", out var msgEl))
                result.Message = msgEl.GetString() ?? "";
        }
        return result;
    }

    public async Task<ClientInfo> ClientRegisterAsync(string name, string phone, string password)
    {
        var resp = await PostJsonRawAsync("/api/agent/client-register", new { pc_id = _cfg.PcId, name, phone, password });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        var obj = JsonSerializer.Deserialize<Dictionary<string, JsonElement>>(json);
        if (obj != null && obj.TryGetValue("client", out var clientEl))
            return clientEl.Deserialize<ClientInfo>() ?? new ClientInfo();
        return new ClientInfo();
    }

    public async Task<Dictionary<string, JsonElement>> ClientLogoutAsync()
    {
        var resp = await PostJsonRawAsync("/api/agent/client-logout", new { pc_id = _cfg.PcId });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        return JsonSerializer.Deserialize<Dictionary<string, JsonElement>>(json) ?? new Dictionary<string, JsonElement>();
    }

    public async Task CallAdminAsync(string reason)
    {
        await PostJsonAsync("/api/agent/call", new { pc_id = _cfg.PcId, reason });
    }

    public async Task<Dictionary<string, JsonElement>> BuyAsync(long productId, long qty = 1)
    {
        var resp = await PostJsonRawAsync("/api/agent/buy", new { pc_id = _cfg.PcId, product_id = productId, qty });
        resp.EnsureSuccessStatusCode();
        var json = await resp.Content.ReadAsStringAsync();
        return JsonSerializer.Deserialize<Dictionary<string, JsonElement>>(json) ?? new Dictionary<string, JsonElement>();
    }

    public async Task<List<ProductInfo>> GetProductsAsync()
    {
        try
        {
            using var req = new HttpRequestMessage(HttpMethod.Get, $"{Base}/agent/products/{_cfg.PcId}");
            req.Headers.Add("X-PC-Token", _cfg.PcToken);
            using var resp = await _http.SendAsync(req);
            resp.EnsureSuccessStatusCode();
            var json = await resp.Content.ReadAsStringAsync();
            var obj = JsonSerializer.Deserialize<Dictionary<string, JsonElement>>(json);
            if (obj != null && obj.TryGetValue("products", out var productsEl))
                return productsEl.Deserialize<List<ProductInfo>>() ?? new List<ProductInfo>();
            return new List<ProductInfo>();
        }
        catch
        {
            return new List<ProductInfo>();
        }
    }

    public async Task<bool> ExitAuthAsync(string password)
    {
        try
        {
            var resp = await PostJsonRawAsync("/api/agent/exit-auth", new { password });
            return resp.IsSuccessStatusCode;
        }
        catch
        {
            return false;
        }
    }

    /// <summary>Скачивает фоновое изображение с сервера (null — фона нет).</summary>
    public async Task<byte[]> GetBackgroundAsync()
    {
        try
        {
            using var req = new HttpRequestMessage(HttpMethod.Get, $"{Base}/api/background");
            using var resp = await _http.SendAsync(req);
            if (!resp.IsSuccessStatusCode) return null;
            return await resp.Content.ReadAsByteArrayAsync();
        }
        catch
        {
            return null;
        }
    }

    /// <summary>Скачивает кастомную обложку по имени (null — нет). Имя — как имя ярлыка в нижнем регистре.</summary>
    public async Task<byte[]> GetCustomCoverAsync(string name)
    {
        try
        {
            using var req = new HttpRequestMessage(HttpMethod.Get, $"{Base}/api/covers/{Uri.EscapeDataString(name)}");
            using var resp = await _http.SendAsync(req);
            if (!resp.IsSuccessStatusCode) return null;
            return await resp.Content.ReadAsByteArrayAsync();
        }
        catch
        {
            return null;
        }
    }

    private async Task<HttpResponseMessage> PostJsonRawAsync(string path, object body)
    {
        using var req = new HttpRequestMessage(HttpMethod.Post, Base + path);
        req.Headers.Add("X-PC-Token", _cfg.PcToken);
        req.Content = new StringContent(JsonSerializer.Serialize(body), Encoding.UTF8, "application/json");
        return await _http.SendAsync(req);
    }

    private async Task PostJsonAsync(string path, object body)
    {
        using var resp = await PostJsonRawAsync(path, body);
        resp.EnsureSuccessStatusCode();
    }
}
