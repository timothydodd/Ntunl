using System.IO.Compression;
using System.Security.Cryptography;
using System.Security.Cryptography.X509Certificates;
using Microsoft.Extensions.Logging;

namespace NtunlCommon;
public static class Utility
{
    public static byte[] CompressData(byte[] data, EncodeType encoding)
    {
        if (encoding == EncodeType.Brotli)
        {
            return BrotliCompress(data);
        }
        else if (encoding == EncodeType.Gzip)
        {
            return GzipCompress(data);
        }

        return data;
    }

    public static byte[] BrotliCompress(byte[] data)
    {
        using var ms = new MemoryStream();
        using var bs = new BrotliStream(ms, CompressionMode.Compress);
        bs.Write(data, 0, data.Length);
        bs.Close();

        return ms.ToArray();
    }

    public static byte[] GzipCompress(byte[] data)
    {
        using var ms = new MemoryStream();
        using var bs = new GZipStream(ms, CompressionMode.Compress);
        bs.Write(data, 0, data.Length);
        bs.Close();
        return ms.ToArray();
    }

    public static byte[] BrotliDecompress(byte[] data)
    {
        using var input = new MemoryStream(data);
        using var output = new MemoryStream();

        using (var bs = new BrotliStream(input, CompressionMode.Decompress))
        {
            bs.CopyTo(output);
        }

        return output.ToArray();
    }

    public static byte[] GzipDecompress(byte[] data)
    {
        using var input = new MemoryStream(data);
        using var output = new MemoryStream();

        using (var bs = new GZipStream(input, CompressionMode.Decompress))
        {
            bs.CopyTo(output);
        }

        return output.ToArray();
    }
    public static string CombineUrlPath(string path, string subpath)
    {
        return $"{path.TrimEnd('/')}/{subpath.TrimStart('/')}";
    }
    public static X509Certificate2 GetOrCreateCertificate(ICertPath certConfig, ILogger logger)
    {
        var path = certConfig.Path;
        var password = certConfig.Password;
        if (File.Exists(path))
        {
            logger.LogInformation("Loading existing certificate...");
            return new X509Certificate2(path, password);
        }
        else
        {
            Console.WriteLine("No certificate found. Generating a new self-signed certificate...");

            var rsa = RSA.Create(2048); // Create an RSA key with 2048-bit length
            var req = new CertificateRequest("CN=localhost", rsa, HashAlgorithmName.SHA256, RSASignaturePadding.Pkcs1);
            req.CertificateExtensions.Add(
                new X509KeyUsageExtension(X509KeyUsageFlags.DigitalSignature, false));

            // Self-sign the certificate
            var cert = req.CreateSelfSigned(DateTimeOffset.Now, DateTimeOffset.Now.AddYears(5));

            // Export the certificate to a PFX file
            File.WriteAllBytes(path, cert.Export(X509ContentType.Pfx, password));

            return cert;
        }

    }
}
public enum EncodeType
{
    None,
    Gzip,
    Brotli
}
