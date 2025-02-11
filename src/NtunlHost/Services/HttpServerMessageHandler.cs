using System.Collections.Specialized;
using System.Net;
using System.Text;
using Microsoft.Extensions.Options;
using NtunlCommon;

namespace NtunlHost.Services;
public class HttpServerMessageHandler
{
    private readonly TunnelHost _tunnelHost;
    private readonly ILogger<HttpServerMessageHandler> _logger;
    private readonly List<string> _headerBlacklistWild = new List<string>();
    private readonly HashSet<string> _headerBlacklist = new HashSet<string>(StringComparer.OrdinalIgnoreCase);
    private readonly string _ipHeaderName = "";
    private readonly int _defaultResponseCode;
    public HttpServerMessageHandler(
        TunnelHost tunnelHost,
        ILogger<HttpServerMessageHandler> logger,
        IOptions<HttpHostSetting> httpHostSettings)
    {
        _tunnelHost = tunnelHost;
        _logger = logger;
        var headerSettings = httpHostSettings.Value.Headers;

        InitializeHeaderBlacklist(headerSettings);
        _ipHeaderName = headerSettings.IpHeaderName ?? "X-Forwarded-For";
        _defaultResponseCode = httpHostSettings.Value.DefaultResponseCode;
    }
    private void InitializeHeaderBlacklist(HttpHostHeaderSettings httpHostHeaderSettings)
    {
        var items = httpHostHeaderSettings.BlackList ?? new List<string>();
        foreach (var item in items)
        {
            var isWild = item.Contains("*");
            if (isWild)
                _headerBlacklistWild.Add(item.TrimEnd('*'));
            else
                _headerBlacklist.Add(item);
        }
    }

    public async Task ProcessRequestAsync(HttpListenerContext ctx)
    {



        //find relative path in url

        var path = ctx.Request.Url.AbsolutePath;
        //get subdomain from url
        var clientName = ctx.Request.Url.Host.Split('.')[0];
        //get request path without host

        ClientInfo? client;
        if (clientName == "localhost" || clientName == "192")
        {
            client = _tunnelHost.GetAnyClient();
        }
        else
        {
            client = _tunnelHost.GetClient(clientName);
        }


        if (client == null)
        {
            ctx.Response.StatusCode = _defaultResponseCode;
            ctx.Response.Close();
            return;
        }



        string clientIp;
        HttpRequestData httpRequestData = new HttpRequestData
        {
            Method = ctx.Request.HttpMethod,
            Path = path,
            Content = await StreamToBytes(ctx.Request.InputStream),
            ContentHeaders = new Dictionary<string, string>(){
                    { "Content-Type", ctx.Request.ContentType ?? "application/octet-stream" },
                    { "Content-Length", ctx.Request.ContentLength64.ToString() },
                    {"Content-Encoding"    ,ctx.Request.ContentEncoding.ToString() ??"" }

            },
            Headers = GetHeadersDictionary(ctx.Request.Headers, out clientIp)
        };

        _logger.LogInformation("{ClientIp} => {method}: {Path}", clientIp, ctx.Request.HttpMethod, path);

        var httpResponse = await _tunnelHost.SendHttpRequest(httpRequestData, client, 20000);


        foreach (var header in httpResponse.Headers)
        {
            ctx.Response.Headers.Add(header.Key, header.Value);
        }

        byte[]? buf = httpResponse.Content;


        if (httpResponse.ContentHeaders.ContainsKey("Content-Encoding"))
        {

            ctx.Response.Headers["Content-Encoding"] = httpResponse.ContentHeaders["Content-Encoding"];


        }

        if (buf == null)
        {
            ctx.Response.StatusCode = 500;
            ctx.Response.Close();
            return;
        }
        ctx.Response.StatusCode = (int)httpResponse.StatusCode;
        ctx.Response.ContentEncoding = Encoding.UTF8;
        ctx.Response.ContentType = httpResponse.ContentHeaders.ContainsKey("Content-Type") ? httpResponse.ContentHeaders["Content-Type"] : "application/octet-stream";

        try
        {
            ctx.Response.OutputStream.Write(buf, 0, buf.Length);
        }
        catch (Exception ex)
        {
            ctx.Response.StatusCode = 500;
            _logger.LogError(ex, $"Error writing response {httpRequestData.Method} => {path}");
        }

        ctx.Response.Close();
        //LogRequest(httpRequestData);
        //LogResponse(httpResponse);



    }
    private async Task<byte[]> StreamToBytes(Stream input)
    {
        using (var memoryStream = new MemoryStream())
        {
            await input.CopyToAsync(memoryStream);
            return memoryStream.ToArray();
        }
    }
    private Dictionary<string, string> GetHeadersDictionary(NameValueCollection headers, out string clientIp)
    {
        clientIp = "unknown";
        var headersDict = new Dictionary<string, string>();
        foreach (string? key in headers.AllKeys)
        {
            if (string.IsNullOrEmpty(key))
                continue;
            if (_headerBlacklist.Count > 0 && _headerBlacklist.Contains(key))
                continue;

            if (key == _ipHeaderName)
            {
                clientIp = headers[key];
                continue;
            }
            bool found = false;
            for (int i = 0; i < _headerBlacklistWild.Count; i++)
            {
                if (key.StartsWith(_headerBlacklistWild[i], StringComparison.InvariantCultureIgnoreCase))
                { found = true; break; }
            }
            if (found)
                continue;



            headersDict[key] = headers[key];
        }


        return headersDict;
    }
}
