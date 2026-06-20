using System.Net.Security;
using System.Security.Cryptography.X509Certificates;
using System.Text.Json;
using Microsoft.Extensions.Options;
using NtunlCommon;
using WatsonWebsocket;

namespace NtunlHost.Services;
public class TunnelHost : IDisposable
{
    readonly WatsonWsServer _server;
    readonly ILogger<TunnelHost> _logger;
    private readonly TunnelHostSettings _hostSettings;
    readonly Dictionary<string, ClientInfo> clientsByName = new Dictionary<string, ClientInfo>();
    private readonly string? domain;
    private readonly string[] subDomains;
    private event EventHandler<SyncMessageReceivedEventArgs>? _syncMessageReceived;
    private readonly object _syncResponseLock = new object();
    public TunnelHost(ILogger<TunnelHost> logger, IOptions<TunnelHostSettings> hostSettings)
    {
        _logger = logger;
        _hostSettings = hostSettings.Value;
        domain = _hostSettings.ClientDomain.Domain;
        subDomains = _hostSettings.ClientDomain.SubDomains.ToArray();
        _server = new WatsonWsServer(_hostSettings.HostName, _hostSettings.Port, _hostSettings.SslSettings?.Enabled == true);

    }
    public void Start()
    {


        _server.ClientConnected += ClientServerConnected;
        _server.ClientDisconnected += ClientDisconnected;
        _server.MessageReceived += MessageReceived;

        _server.Start();
        _logger.LogInformation("TcpServer is running.");


    }

    private bool CertificateValidationCallback(
    object sender,
    X509Certificate? certificate,
    X509Chain? chain,
    SslPolicyErrors sslPolicyErrors)
    {
        // Log the certificate details
        LogCertificateDetails(certificate);

        // Custom validation logic (optional)
        if (sslPolicyErrors == SslPolicyErrors.None)
        {
            _logger.LogInformation("Certificate is valid.");
            return true; // Certificate is valid
        }
        else
        {
            _logger.LogError($"Certificate error: {sslPolicyErrors}");
            return _hostSettings?.SslSettings?.AcceptInvalidCertificates == true;
        }
    }

    private void LogCertificateDetails(X509Certificate certificate)
    {
        if (certificate == null)
        {
            _logger.LogInformation("No certificate provided.");
            return;
        }

        _logger.LogInformation("Certificate Details:");
        _logger.LogInformation($"- Subject: {certificate.Subject}");
        _logger.LogInformation($"- Issuer: {certificate.Issuer}");
        _logger.LogInformation($"- Effective Date: {certificate.GetEffectiveDateString()}");
        _logger.LogInformation($"- Expiration Date: {certificate.GetExpirationDateString()}");
        _logger.LogInformation($"- Thumbprint: {certificate.GetCertHashString()}");
    }

    void ClientServerConnected(object? sender, ConnectionEventArgs args)
    {
        var name = GetRandomName();

        if (name == null)
        {
            this.SendMessage(CommandType.Echo, args.Client.Guid, "No more subdomains available").GetAwaiter().GetResult();
            _server?.DisconnectClient(args.Client.Guid);
            return;
        }
        args.Client.Name = name;
        clientsByName[args.Client.Name.ToLower()] = new ClientInfo { Id = args.Client.Guid, Name = args.Client.Name };

        if (!string.IsNullOrWhiteSpace(domain))
        {
            this.SendMessage(CommandType.NtunlInfo, new NtunlInfo()
            {
                Url = $"https://{args.Client.Name}.{domain}",
            }, args.Client.Guid).GetAwaiter().GetResult();
        }
        _logger.LogInformation("Client connected: " + args.Client.ToString());

    }

    void ClientDisconnected(object? sender, DisconnectionEventArgs args)
    {
        if (args.Client.Name != null)
        {
            clientsByName.Remove(args.Client.Name.ToLower());
            _logger.LogInformation("Client disconnected: " + args.Client.ToString());
        }
    }

    void MessageReceived(object? sender, MessageReceivedEventArgs args)
    {
        lock (_syncResponseLock)
        {
            this._syncMessageReceived?.Invoke(this, new SyncMessageReceivedEventArgs() { Command = Command.FromBytes(args.Data.ToArray()) });
        }
    }
    public ClientInfo? GetClient(string name)
    {
        if (clientsByName.ContainsKey(name.ToLower()))
        {
            return clientsByName[name.ToLower()];
        }
        return null;
    }
    public ClientInfo? GetAnyClient()
    {
        return clientsByName.Values.FirstOrDefault();
    }
    public async Task SendMessage<T>(CommandType t, T message, Guid id)
    {
        var command = new Command
        {
            ConversationId = Guid.NewGuid(),
            CommandType = t,
            Data = JsonSerializer.Serialize(message, DefaultSettings.JsonSerializerOptions)
        };
        var serialized = Command.ToBytes(command);
        await _server.SendAsync(id, serialized);
    }
    private async Task SendMessage(CommandType t, Guid id, string message)
    {
        if (_server == null)
            throw new Exception("Server not started");

        var command = new Command
        {
            ConversationId = Guid.NewGuid(),
            CommandType = t,
            Data = message
        };
        var serialized = Command.ToBytes(command);
        await _server.SendAsync(id, serialized);

    }
    public async Task<HttpResponseData> SendHttpRequest(HttpRequestData message, ClientInfo client, int timeoutMs = 151000)
    {
        if (_server == null)
            throw new Exception("Server not started");

        await client.WriteLock.WaitAsync();

        var command = new Command
        {
            ConversationId = Guid.NewGuid(),
            CommandType = CommandType.HttpRequest,
            Data = JsonSerializer.Serialize(message, DefaultSettings.JsonSerializerOptions)
        };


        var id = client.Id;

        AutoResetEvent responded = new AutoResetEvent(false);

        HttpResponseData? response = null;

        EventHandler<SyncMessageReceivedEventArgs> handler = (sender, e) =>
        {


            if (e.Command.ConversationId == command.ConversationId)
            {
                response = e.Command.Data == null ? null : JsonSerializer.Deserialize<HttpResponseData>(e.Command.Data, DefaultSettings.JsonSerializerOptions);
                responded.Set();
            }
        };
        _syncMessageReceived += handler;

        try
        {
            var serialized = Command.ToBytes(command);

            await _server.SendAsync(id, serialized);
            _logger.LogDebug("HttpRequest sent to {id}", id);
        }
        catch (Exception e)
        {
            _logger.LogError(" failed to write message: " + e.Message);
            _syncMessageReceived -= handler;
            throw;
        }
        finally
        {
            if (client != null)
                client.WriteLock.Release();
        }
        responded.WaitOne(new TimeSpan(0, 0, 0, 0, timeoutMs));
        _syncMessageReceived -= handler;

        if (response != null)
        {
            return response;
        }
        else
        {
            _logger.LogError("synchronous response not received within the timeout window");
            throw new TimeoutException("A response to a synchronous request was not received within the timeout window.");
        }

    }


    readonly List<string> words = new List<string> { "apple", "banana", "cherry", "date", "elderberry", "fig", "grape", "honeydew", "alpha", "beta", "gamma", "delta", "epsilon", "zeta", "eta", "theta" };
    private string? GetRandomName()
    {
        if (subDomains.Length > 0)
        {
            for (int i = 0; i < subDomains.Length; i++)
            {
                if (!clientsByName.ContainsKey(subDomains[i]))
                {
                    return subDomains[i];
                }
            }
            return null;
        }
        var word = "";
        do
        {
            Random random = new Random();
            int wordIndex = random.Next(words.Count);
            int randomNumber = random.Next(1, 1000); // Generates a random number between 1 and 999


            word = $"{words[wordIndex]}{randomNumber}";
        } while (clientsByName.ContainsKey(word));
        return word;
    }

    public void Dispose()
    {
        if (_server != null)
        {
            _server.Dispose();
        }
    }
}
public class SyncMessageReceivedEventArgs : EventArgs
{
    public required Command Command { get; set; }

}
public class ClientInfo
{
    public Guid Id { get; set; }
    public SemaphoreSlim WriteLock = new SemaphoreSlim(1);
    public required string Name { get; set; }
}
