using System.Net.Http.Headers;
using System.Text;
using System.Text.Json;
using System.Text.RegularExpressions;
using Microsoft.Extensions.Options;
using NtunlClient.Common;
using NtunlCommon;

namespace NtunlClient.Services;
public class ClientMessageHandler
{
    private readonly IHttpClientFactory _httpClientFactory;
    public List<RequestLog> RequestLogs = new List<RequestLog>();
    readonly ILogger<ClientMessageHandler> _logger;

    public ClientMessageHandler(ILogger<ClientMessageHandler> logger, IHttpClientFactory httpClientFactory, IOptions<TunnelSetting> options)
    {
        _logger = logger;
        _httpClientFactory = httpClientFactory;
    }



    public async Task<Command> HandleMessage(Command cmd, TunnelState state)
    {

        if (cmd == null)
            throw new ArgumentNullException(nameof(cmd));
        if (state == null)
            throw new ArgumentNullException(nameof(state));

        switch (cmd.CommandType)
        {
            case CommandType.HttpRequest:
                {
                    if (string.IsNullOrWhiteSpace(cmd.Data))
                    {
                        throw new Exception("No data in request");
                    }

                    var httpRequest = JsonSerializer.Deserialize<HttpRequestData>(cmd.Data, DefaultSettings.JsonSerializerOptions)
                        ?? throw new Exception("Failed to deserialize HttpRequestData.");


                    var client = _httpClientFactory.CreateClient();
                    var method = GetMethod(httpRequest.Method);
                    var requestUri = new Uri(Utility.CombineUrlPath(state.HostSettings.Address, httpRequest.Path));

                    using var request = new HttpRequestMessage(method, requestUri);
                    _logger.LogDebug("{method}: {Path}", httpRequest.Method, httpRequest.Path);


                    foreach (var header in httpRequest.Headers)
                    {
                        if (header.Key == "Host" && !string.IsNullOrWhiteSpace(state.HostSettings.HostHeader))
                        {
                            request.Headers.Host = state.HostSettings.HostHeader;
                            continue;
                        }

                        if (header.Key == "Content-Length" || header.Key == "Content-Type")
                        {
                            continue;
                        }

                        try
                        {
                            request.Headers.TryAddWithoutValidation(header.Key, header.Value);
                        }
                        catch (Exception ex)
                        {
                            _logger.LogError(ex, "Error adding header: {HeaderKey}", header.Key);
                        }
                    }

                    if (httpRequest.Content != null && httpRequest.Content.Length > 0)
                    {
                        request.Content = new ByteArrayContent(httpRequest.Content);

                        foreach (var header in httpRequest.ContentHeaders)
                        {
                            request.Content.Headers.TryAddWithoutValidation(header.Key, header.Value);
                        }
                    }

                    using var response = await client.SendAsync(request).ConfigureAwait(false);
                    var httpResponse = await CreateResponseDataAsync(response, state).ConfigureAwait(false);

                    var command = new Command
                    {
                        ConversationId = cmd.ConversationId,
                        CommandType = CommandType.HttpResponse,
                        Data = JsonSerializer.Serialize(httpResponse, DefaultSettings.JsonSerializerOptions)
                    };

                    RequestLogs.Add(new RequestLog { Request = httpRequest, Response = httpResponse });

                    return command;
                }
        }

        throw new ArgumentException("Unknown command type.");
    }

    private Dictionary<string, string> ConvertToDictionary(HttpResponseHeaders collection)
    {
        var dict = new Dictionary<string, string>();

        foreach (var kv in collection)
        {
            dict.Add(kv.Key, string.Join(',', kv.Value));
        }

        return dict;
    }

    private Dictionary<string, string> ConvertToDictionary(HttpContentHeaders collection)
    {
        var dict = new Dictionary<string, string>();

        foreach (var kv in collection)
        {
            dict.Add(kv.Key, string.Join(',', kv.Value));
        }

        return dict;
    }

    private async Task<HttpResponseData> CreateResponseDataAsync(HttpResponseMessage response, TunnelState state)
    {
        var httpResponse = new HttpResponseData
        {
            Headers = ConvertToDictionary(response.Headers),
            StatusCode = response.StatusCode,
            Content = await response.Content.ReadAsByteArrayAsync(),
            ContentHeaders = ConvertToDictionary(response.Content.Headers)
        };

        if (state.UrlRewriteEnabled && httpResponse.ContentHeaders.TryGetValue("Content-Type", out string? contentType) && contentType != null && contentType.Contains("text/html") && state.UrlRewriteRegex != null && state.NtunlInfo?.Url != null)
        {
            var encodeType = DecompressContent(httpResponse);
            var content = Encoding.UTF8.GetString(httpResponse.Content);
            content = state.UrlRewriteRegex.Replace(content, state.NtunlInfo.Url);
            httpResponse.Content = Utility.CompressData(Encoding.UTF8.GetBytes(content), encodeType);
            httpResponse.ContentHeaders["Content-Length"] = httpResponse.Content.Length.ToString();
        }

        return httpResponse;
    }



    private EncodeType DecompressContent(HttpResponseData data)
    {
        if (data.ContentHeaders.Count > 0 && data.ContentHeaders.ContainsKey("Content-Encoding"))
        {
            if (data.ContentHeaders["Content-Encoding"] == "br")
            {
                data.Content = Utility.BrotliDecompress(data.Content);
                return EncodeType.Brotli;
            }
            else if (data.ContentHeaders["Content-Encoding"] == "gzip")
            {
                data.Content = Utility.GzipDecompress(data.Content);
                return EncodeType.Gzip;
            }
        }

        return EncodeType.None;
    }


    private HttpMethod GetMethod(string meth)
    {
        return meth switch
        {
            "POST" => HttpMethod.Post,
            "PUT" => HttpMethod.Put,
            "DELETE" => HttpMethod.Delete,
            _ => HttpMethod.Get
        };
    }
}
public class TunnelState
{
    public readonly TunnelSetting HostSettings;
    public readonly Regex? UrlRewriteRegex = null;
    public readonly bool UrlRewriteEnabled = false;
    public NtunlInfo? NtunlInfo;

    public TunnelState(TunnelSetting hostSettings)
    {
        HostSettings = hostSettings;

        if (HostSettings.RewriteUrlEnabled == true && !string.IsNullOrWhiteSpace(HostSettings.RewriteUrlPattern))
        {
            UrlRewriteRegex = new Regex(HostSettings.RewriteUrlPattern, RegexOptions.Compiled);
            UrlRewriteEnabled = true;
        }
    }
}
