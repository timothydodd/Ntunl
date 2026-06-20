using System.Net.Security;
using System.Security.Cryptography.X509Certificates;
using System.Text.Json;
using NtunlClient.Common;
using NtunlCommon;
using WatsonWebsocket;

namespace NtunlClient.Services;
public class TunnelClient
{

    private readonly WatsonWsClient _client;
    private readonly ILogger<TunnelClient> _logger;
    private readonly ClientMessageHandler _clientMessageHandler;
    private readonly TunnelState _state;

    public TunnelClient(ILogger<TunnelClient> logger,
        ClientMessageHandler clientMessageHandler, TunnelSetting settings)
    {


        _logger = logger;
        _clientMessageHandler = clientMessageHandler;
        _state = new TunnelState(settings);
        _client = CreateTcpClient();
        ConfigureClient();

    }

    private void ClientServerDisconnected(object? sender, EventArgs e)
    {
        _logger.LogInformation("Client: disconnected from server");
    }

    private void ClientServerConnected(object? sender, EventArgs e)
    {

        _logger.LogInformation("Client: connected to server");

    }
    private void ClientMessageReceived(object? sender, MessageReceivedEventArgs e)
    {


        Command cmd = Command.FromBytes(e.Data.ToArray());
        switch (cmd.CommandType)
        {
            case CommandType.Echo:
                {
                    _logger.LogInformation(cmd.Data);
                    break;
                }
            case CommandType.NtunlInfo:
                {
                    _state.NtunlInfo = JsonSerializer.Deserialize<NtunlInfo>(cmd.Data, DefaultSettings.JsonSerializerOptions);
                    _logger.LogInformation($"Your Url: {_state.NtunlInfo?.Url}");
                    break;
                }
            case CommandType.HttpRequest:
                {
                    try
                    {
                        var response = _clientMessageHandler.HandleMessage(cmd, _state).GetAwaiter().GetResult();

                        _client.SendAsync(Command.ToBytes(response)).GetAwaiter().GetResult();
                    }
                    catch (Exception ex)
                    {
                        _logger.LogError(ex, "Error handling message");
                    }
                    break;
                }
        }
    }


    public async Task Start(CancellationToken stoppingToken)
    {
        int retryInterval = 5000; // 5 seconds between retries
        int maxRetries = 10; // Maximum number of retries, set to -1 for unlimited retries
        int retryCount = 0;
        bool connected = false;

        while (!connected && (retryCount < maxRetries || maxRetries == -1) && !stoppingToken.IsCancellationRequested)
        {
            try
            {
                this.Connect();
                connected = true;
            }
            catch (Exception ex)
            {
                retryCount++;
                _logger.LogWarning($"Connection attempt {retryCount} failed: {ex.Message}");

                if (retryCount >= maxRetries && maxRetries != -1)
                {
                    _logger.LogError("Maximum retry attempts reached. Exiting.");
                    return;
                }

                await Task.Delay(retryInterval, stoppingToken); // Wait before retrying
            }
        }


    }
    public void Stop()
    {
        _client?.Dispose();
    }
    private WatsonWsClient CreateTcpClient()
    {
        var address = _state.HostSettings.NtunlAddress.Split(':');
        int serverPort = int.Parse(address[1]);
        string ip = address[0];

        return new WatsonWsClient(ip, serverPort, _state.HostSettings.SslEnabled);

    }
    private void ConfigureClient()
    {

        _client.ServerConnected += ClientServerConnected;
        _client.ServerDisconnected += ClientServerDisconnected;
        _client.MessageReceived += ClientMessageReceived;
        _client.ConfigureOptions((options) =>
        {
            options.RemoteCertificateValidationCallback = CertificateValidationCallback;
        });
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
            return _state.HostSettings.AllowInvalidCertificates;
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
    private void Connect()
    {
        _client.Start();
    }
}

public class RequestLog
{
    public required HttpRequestData Request { get; set; }
    public required HttpResponseData Response { get; set; }
}
