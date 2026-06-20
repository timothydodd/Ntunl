using System.Net;
using Microsoft.Extensions.Options;

namespace NtunlHost.Services;

public class HttpServer : BackgroundService, IDisposable
{
    private HttpListener? _httpListener;
    private readonly ILogger<HttpServer> _logger;
    private readonly HttpHostSetting _hostSettings;
    private readonly HttpServerMessageHandler _messageHandler;


    public HttpServer(ILogger<HttpServer> logger,
        IOptions<HttpHostSetting> hostSettings,
        HttpServerMessageHandler messageHandler)
    {
        _logger = logger;
        _hostSettings = hostSettings.Value;
        _messageHandler = messageHandler;

    }
    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        const int maxConcurrentRequests = 5; // Set the maximum number of concurrent requests.
        var semaphore = new SemaphoreSlim(maxConcurrentRequests);

        _httpListener = new HttpListener();
        _httpListener.Prefixes.Add($"http://{_hostSettings.HostName}:{_hostSettings.Port}/");

        _httpListener.Start();
        _logger.LogInformation("HttpServer is running.");


        try
        {
            while (!stoppingToken.IsCancellationRequested)
            {
                HttpListenerContext ctx;
                try
                {
                    ctx = await _httpListener.GetContextAsync().ConfigureAwait(false);
                }
                catch (HttpListenerException) when (stoppingToken.IsCancellationRequested)
                {
                    // Listener was stopped due to cancellation, so we can safely exit the loop.
                    break;
                }
                catch (Exception ex)
                {
                    _logger.LogError(ex, "Error occurred while trying to get HTTP context.");
                    continue;
                }

                // Wait for an available slot to process the request.
                await semaphore.WaitAsync(stoppingToken);

                _ = Task.Run(async () =>
                {
                    try
                    {
                        await _messageHandler.ProcessRequestAsync(ctx).ConfigureAwait(false);
                    }
                    catch (Exception ex)
                    {
                        _logger.LogError(ex, "Error occurred while processing request.");
                    }
                    finally
                    {
                        semaphore.Release(); // Release the slot for another request.
                    }
                }, stoppingToken);
            }
        }
        finally
        {
            _httpListener.Stop();
            _httpListener.Close();
            _logger.LogInformation("HttpServer has stopped.");
        }


    }


}
