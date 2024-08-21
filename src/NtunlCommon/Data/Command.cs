using System.Net;
using System.Text.Json;
using System.Text.Json.Serialization;

namespace NtunlCommon;
public enum CommandType
{
    Echo = 1,
    HttpRequest = 2,
    HttpResponse = 3,
    NtunlInfo = 4
}


public class Command
{
    public required CommandType CommandType { get; set; } = CommandType.Echo;
    public required Guid ConversationId { get; set; }

    public required string Data { get; set; } = "";

    public Command()
    {

    }
    public static byte[] ToBytes(Command command)
    {
        BinaryWriter writer = new BinaryWriter(new MemoryStream());

        writer.Write((int)command.CommandType);
        writer.Write(command.ConversationId.ToByteArray());
        writer.Write(command.Data ?? "");

        return ((MemoryStream)writer.BaseStream).ToArray();

    }
    public static Command FromBytes(byte[] data)
    {
        BinaryReader reader = new BinaryReader(new MemoryStream(data));
        var type = (CommandType)reader.ReadInt32();
        var guidBytes = reader.ReadBytes(16);
        var guid = new Guid(guidBytes);
        var dataField = reader.ReadString();


        return new Command()
        {
            CommandType = type,
            ConversationId = guid,
            Data = dataField
        };
    }


}
public class HttpRequestData
{
    public Dictionary<string, string> Headers { get; set; } = new Dictionary<string, string>();
    public required string Method { get; set; }
    public required string Path { get; set; }
    public byte[]? Content { get; set; }
    public Dictionary<string, string> ContentHeaders { get; set; } = new Dictionary<string, string>();

}
public class HttpResponseData
{
    public HttpStatusCode StatusCode { get; set; }
    public byte[]? Content { get; set; }
    public Dictionary<string, string> ContentHeaders { get; set; } = new Dictionary<string, string>();
    public Dictionary<string, string> Headers { get; set; } = new Dictionary<string, string>();

}
public class NtunlInfo
{
    public string Url { get; set; } = "";

}

[JsonSerializable(typeof(Command))]
[JsonSerializable(typeof(HttpRequestData))]
[JsonSerializable(typeof(NtunlInfo))]
[JsonSerializable(typeof(HttpResponseData))]
public partial class ApiJsonSerializerContext : JsonSerializerContext
{

}

public static class DefaultSettings
{
    public static JsonSerializerOptions JsonSerializerOptions = new JsonSerializerOptions()
    {
        PropertyNamingPolicy = JsonNamingPolicy.CamelCase,
        WriteIndented = true,
        TypeInfoResolver = new ApiJsonSerializerContext()
      ,
        PropertyNameCaseInsensitive = true
    };
}
