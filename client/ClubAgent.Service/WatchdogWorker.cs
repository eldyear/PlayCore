using Microsoft.Extensions.Hosting;
using Microsoft.Extensions.Logging;

namespace Club.Agent.Service;

/// <summary>
/// Служба-сторож: следит за heartbeat-файлом агента и перезапускает
/// GUI-агента (ClubShell) в сессии пользователя, если тот умер/завис.
/// Работает от SYSTEM, обычный пользователь её убить не может.
/// </summary>
public class WatchdogWorker : BackgroundService
{
    private readonly ILogger<WatchdogWorker> _logger;

    public WatchdogWorker(ILogger<WatchdogWorker> logger) => _logger = logger;

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var shellExe = Path.Combine(AppContext.BaseDirectory, "ClubShell.exe");
        // Общий путь с агентом (Public\Documents), т.к. %TEMP% у SYSTEM и пользователя разный.
        var heartbeat = Path.Combine(Environment.GetFolderPath(Environment.SpecialFolder.CommonDocuments), "gcagent.heartbeat");

        _logger.LogInformation("Watchdog запущен. Агент: {Shell}", shellExe);

        while (!stoppingToken.IsCancellationRequested)
        {
            try
            {
                var stale = true;
                if (File.Exists(heartbeat))
                    stale = (DateTime.Now - File.GetLastWriteTime(heartbeat)).TotalSeconds > 8;

                if (stale)
                {
                    _logger.LogWarning("агент не отвечает — перезапускаю");
                    ProcessLauncher.LaunchInUserSession(shellExe);
                }
            }
            catch (Exception ex)
            {
                _logger.LogError(ex, "ошибка watchdog");
            }

            await Task.Delay(2000, stoppingToken);
        }
    }
}
