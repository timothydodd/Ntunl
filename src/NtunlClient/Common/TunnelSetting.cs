namespace NtunlClient.Common;
public class TunnelSetting
{
    public required string NtunlAddress { get; set; }
    public required string Address { get; set; }
    public required string HostHeader { get; set; }
    public required Dictionary<string, string> CustomHeader { get; set; }
    public bool? RewriteUrlEnabled { get; set; }
    public string? RewriteUrlPattern { get; set; }
    public bool SslEnabled { get; set; }
    public bool AllowInvalidCertificates { get; set; }

}
