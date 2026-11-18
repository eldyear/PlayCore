using Club.Agent.Service;
using Microsoft.Extensions.DependencyInjection;
using Microsoft.Extensions.Hosting;

var builder = Host.CreateApplicationBuilder(args);
builder.Services.AddWindowsService(options =>
{
    options.ServiceName = "GCAgentWatchdog";
});
builder.Services.AddHostedService<WatchdogWorker>();

var host = builder.Build();
await host.RunAsync();
