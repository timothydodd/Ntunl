using System.IO.Compression;
using System.Net;
using System.Text;
using HandlebarsDotNet;

namespace NtunlClient.Services;

public class HttpServer : BackgroundService, IDisposable
{
    HttpListener? _httpListener;
    readonly ILogger<HttpServer> _logger;
    private readonly ClientMessageHandler _messageHandler;
    private readonly IConfiguration _configuration;
    private readonly HandlebarsTemplate<object, object> _template;

    public HttpServer(ILogger<HttpServer> logger, ClientMessageHandler messageHandler, IConfiguration configuration)
    {
        _logger = logger;
        string templateRelativePath = Path.Combine("Templates", "requests.html");
        var templateSource = File.ReadAllText(templateRelativePath);
        _template = Handlebars.Compile(templateSource);
        _messageHandler = messageHandler;
        _configuration = configuration;
    }

    protected override async Task ExecuteAsync(CancellationToken stoppingToken)
    {
        var enabled = _configuration.GetSection("Inspector:Enabled").Get<bool>();
        if (!enabled)
        {
            _logger.LogInformation("Inspector is disabled");
            return;
        }
        var port = _configuration.GetSection("Inspector:Port").Get<int>();

        const int maxConcurrentRequests = 5; // Set the maximum number of concurrent requests.
        var semaphore = new SemaphoreSlim(maxConcurrentRequests);
        var uri = $"http://localhost:{port}/";
        _httpListener = new HttpListener();
        _httpListener.Prefixes.Add(uri);
        _httpListener.Start();

        _logger.LogInformation($"Listening on port: {uri}");

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

                _ = Task.Run(() =>
                {
                    try
                    {
                        ProcessRequest(ctx);

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

    void ProcessRequest(HttpListenerContext ctx)
    {



        ctx.Response.ContentEncoding = Encoding.UTF8;
        ctx.Response.ContentType = "text/html";
        //     ctx.Response.ContentLength64 = buf?.Length ?? 0;

        var data = GetViewModel();

        // Step 4: Render the template with the data
        string resultHtml = _template(data);
        //output result to response
        ctx.Response.StatusCode = 200;
        ctx.Response.ContentLength64 = Encoding.UTF8.GetByteCount(resultHtml);
        try
        {
            ctx.Response.OutputStream.Write(Encoding.UTF8.GetBytes(resultHtml));
        }
        catch (Exception ex)
        {
            _logger.LogError(ex, "Error writing response");
        }

        ctx.Response.Close();

    }
    private HttpLogViewModel GetViewModel()
    {
        var logs = _messageHandler.RequestLogs;
        return new HttpLogViewModel()
        {
            Logs = logs.Select(x =>
            {
                string? content = null;
                if (x.Response.ContentHeaders.ContainsKey("Content-Encoding") && x.Response.ContentHeaders["Content-Encoding"] == "br")
                {
                    content = ConvertBrBufferToUtf8(x.Response?.Content);
                }
                else
                {
                    content = Encoding.UTF8.GetString(x.Response?.Content ?? []);
                }
                var headers = x.Response.Headers ?? new Dictionary<string, string>();
                foreach (var header in x.Response.ContentHeaders)
                {
                    if (!headers.ContainsKey(header.Key))
                        headers.Add(header.Key, header.Value);
                }

                return new HttpLog
                {
                    Request = new HttpRequestLog
                    {
                        Method = x.Request.Method,
                        Url = x.Request.Path,
                        Headers = x.Request.Headers
                    },
                    Response = new HttpResponseLog
                    {
                        StatusCode = (int)x.Response.StatusCode,
                        Headers = x.Response.Headers,
                        Content = content
                    }
                };
            })
        };
    }
    public string ConvertBrBufferToUtf8(byte[]? brBuffer)
    {
        if (brBuffer == null)
            return "";

        using (var inputStream = new MemoryStream(brBuffer))
        using (var decompressionStream = new BrotliStream(inputStream, CompressionMode.Decompress))
        using (var outputStream = new MemoryStream())
        {
            decompressionStream.CopyTo(outputStream);
            byte[] decompressedBytes = outputStream.ToArray();
            return Encoding.UTF8.GetString(decompressedBytes);
        }
    }

}
public class HttpRequestLog
{
    public required string Method { get; set; }
    public required string Url { get; set; }
    public required Dictionary<string, string> Headers { get; set; }
}

public class HttpResponseLog
{
    public int StatusCode { get; set; }
    public required Dictionary<string, string> Headers { get; set; }
    public required string Content { get; set; }
}

public class HttpLog
{
    public required HttpRequestLog Request { get; set; }
    public required HttpResponseLog Response { get; set; }
}
public class HttpLogViewModel
{
    public required IEnumerable<HttpLog> Logs { get; set; }
}
