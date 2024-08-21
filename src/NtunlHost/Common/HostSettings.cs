


using NtunlCommon;

public class HttpHostSetting : HostSetting
{
    public required HttpHostHeaderSettings Headers { get; set; }
}

public class TunnelHostSettings : HostSetting
{
    public required ClientDomainSettings ClientDomain { get; set; }
    public SslSettings? SslSettings { get; set; }
}
public class HostSetting
{
    public required string HostName { get; set; }
    public int Port { get; set; }
}
public class SslSettings
{
    public bool Enabled { get; set; }
    public bool AcceptInvalidCertificates { get; set; }
    public bool MutuallyAuthenticate { get; set; }
    public required CertificateSettings Certificate { get; set; }
}

public class CertificateSettings : ICertPath
{
    public required string Path { get; set; }
    public required string Password { get; set; }
}


public class ClientDomainSettings
{
    public required string Domain { get; set; }
    public required List<string> SubDomains { get; set; }
}
public class HttpHostHeaderSettings
{
    public List<string>? BlackList { get; set; }
    public string? IpHeaderName { get; set; }
}
