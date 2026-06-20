using NtunlCommon;

namespace NtunlHost.Services;

public class TunnelHostWorker : BackgroundService, IDisposable
{
    readonly TunnelHost _server;
    readonly ILogger<TunnelHostWorker> _logger;

    public TunnelHostWorker(ILogger<TunnelHostWorker> logger, TunnelHost server)
    {
        _logger = logger;
        _server = server;

    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        Extensions.DisplayNtunlLogo();

        _server.Start();
        while (!stoppingToken.IsCancellationRequested)
        {
            await Task.Delay(1000, stoppingToken);
        }

    }



}
