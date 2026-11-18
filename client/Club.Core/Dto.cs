using System.Text.Json.Serialization;

namespace Club.Core;

public class PendingCommand
{
    [JsonPropertyName("id")] public long Id { get; set; }
    [JsonPropertyName("kind")] public string Kind { get; set; } = "";
}

public class AgentStatus
{
    [JsonPropertyName("pc_id")] public long PcId { get; set; }
    [JsonPropertyName("pc_name")] public string PcName { get; set; } = "";
    [JsonPropertyName("zone")] public string Zone { get; set; } = "";
    [JsonPropertyName("club_name")] public string ClubName { get; set; } = "";
    [JsonPropertyName("locked")] public bool Locked { get; set; }
    [JsonPropertyName("paused")] public bool Paused { get; set; }
    [JsonPropertyName("remaining_seconds")] public long RemainingSeconds { get; set; }
    [JsonPropertyName("ends_at")] public long EndsAt { get; set; }
    [JsonPropertyName("client_name")] public string ClientName { get; set; } = "";
    [JsonPropertyName("client_id")] public long ClientId { get; set; }
    [JsonPropertyName("client_balance")] public double ClientBalance { get; set; }
    [JsonPropertyName("client_bonus_points")] public long ClientBonusPoints { get; set; }
    [JsonPropertyName("session_started_at")] public long SessionStartedAt { get; set; }
    [JsonPropertyName("session_open")] public bool SessionOpen { get; set; }
    [JsonPropertyName("server_time")] public long ServerTime { get; set; }
    [JsonPropertyName("background_version")] public long BackgroundVersion { get; set; }
    [JsonPropertyName("commands")] public List<PendingCommand> Commands { get; set; } = new();
    [JsonPropertyName("messages")] public List<PendingMessage> Messages { get; set; } = new();
}

public class ClientInfo
{
    [JsonPropertyName("id")] public long Id { get; set; }
    [JsonPropertyName("name")] public string Name { get; set; } = "";
    [JsonPropertyName("phone")] public string Phone { get; set; } = "";
    [JsonPropertyName("balance")] public double Balance { get; set; }
    [JsonPropertyName("bonus_points")] public long BonusPoints { get; set; }
    [JsonPropertyName("discount_percent")] public double DiscountPercent { get; set; }
}

public class ClientLoginResult
{
    public ClientInfo Client { get; set; } = new();
    public bool SessionStarted { get; set; }
    public string Message { get; set; } = "";
}

public class ProductInfo
{
    [JsonPropertyName("id")] public long Id { get; set; }
    [JsonPropertyName("name")] public string Name { get; set; } = "";
    [JsonPropertyName("category")] public string Category { get; set; } = "";
    [JsonPropertyName("price")] public double Price { get; set; }
    [JsonPropertyName("stock")] public long Stock { get; set; }
}

public class PendingMessage
{
    [JsonPropertyName("id")] public long Id { get; set; }
    [JsonPropertyName("message")] public string Message { get; set; } = "";
}

public class ComplianceReport
{
    [JsonPropertyName("windows")] public string Windows { get; set; } = "unknown";
    [JsonPropertyName("windows_ok")] public bool WindowsOk { get; set; }
    [JsonPropertyName("tpm")] public string Tpm { get; set; } = "unknown";
    [JsonPropertyName("tpm_ok")] public bool TpmOk { get; set; }
    [JsonPropertyName("secure_boot")] public string SecureBoot { get; set; } = "unknown";
    [JsonPropertyName("secure_boot_ok")] public bool SecureBootOk { get; set; }
    [JsonPropertyName("vbs")] public string Vbs { get; set; } = "unknown";
    [JsonPropertyName("vbs_ok")] public bool VbsOk { get; set; }
    [JsonPropertyName("test_signing")] public string TestSigning { get; set; } = "unknown";
    [JsonPropertyName("test_signing_ok")] public bool TestSigningOk { get; set; }
    [JsonPropertyName("verdict")] public string Verdict { get; set; } = "unknown";
    [JsonPropertyName("checked_at")] public long CheckedAt { get; set; }
}

public class DiskInfo
{
    [JsonPropertyName("name")] public string Name { get; set; } = "";
    [JsonPropertyName("total")] public long Total { get; set; }
    [JsonPropertyName("free")] public long Free { get; set; }
}

public class HardwareReport
{
    [JsonPropertyName("cpu")] public string Cpu { get; set; } = "";
    [JsonPropertyName("ram")] public string Ram { get; set; } = "";
    [JsonPropertyName("gpu")] public string Gpu { get; set; } = "";
    [JsonPropertyName("disks")] public List<DiskInfo> Disks { get; set; } = new();
}
