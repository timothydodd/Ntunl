using NtunlClient.Common;
using NtunlCommon;

namespace NtunlClient.Services;
internal class TunnelWorker : BackgroundService
{
    private readonly ILogger<TunnelWorker> _logger;
    private readonly List<TunnelClient> _tunnelClients = new List<TunnelClient>();
    public TunnelWorker(ILogger<TunnelWorker> logger, IConfiguration configuration, IServiceProvider serviceProvider)
    {
        _logger = logger;
        var settings = configuration.GetRequiredSection("Tunnels").Get<List<TunnelSetting>>() ?? new List<TunnelSetting>();

        foreach (var setting in settings)
        {
            serviceProvider.CreateScope();
            var tcLogger = serviceProvider.GetRequiredService<ILogger<TunnelClient>>();
            var clientMessageHandler = serviceProvider.GetRequiredService<ClientMessageHandler>();

            _tunnelClients.Add(new TunnelClient(tcLogger, clientMessageHandler, setting));
        }

    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        Extensions.DisplayNtunlLogo();
        _logger.LogInformation("TunnelWorker running at: {time}", DateTimeOffset.Now);


        foreach (var client in _tunnelClients)
        {
            await client.Start(stoppingToken);
        }


        while (!stoppingToken.IsCancellationRequested)
        {
            await Task.Delay(1000, stoppingToken);
        }
        foreach (var client in _tunnelClients)
        {
            client.Stop();
        }
    }
}
