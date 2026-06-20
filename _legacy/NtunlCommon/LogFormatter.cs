using Microsoft.Extensions.Logging;
using Microsoft.Extensions.Logging.Abstractions;
using Microsoft.Extensions.Logging.Console;

namespace NtunlCommon;

public class LogFormatter : ConsoleFormatter
{
    public LogFormatter() : base(nameof(LogFormatter)) { }

    public override void Write<TState>(in LogEntry<TState> logEntry, IExternalScopeProvider scopeProvider, TextWriter textWriter)
    {
        var timestamp = DateTime.Now.ToString("HH:mm:ss ");
        textWriter.Write(timestamp);


        string level = "";
        string? color = null;
        switch (logEntry.LogLevel)
        {
            case LogLevel.Trace:
                level = "trace: ";
                color = "cyan";
                break;
            case LogLevel.Debug:
                color = "cyan";
                level = "debug: ";
                break;
            case LogLevel.Information:
                color = "green";
                level = "info: ";
                break;
            case LogLevel.Warning:
                color = "yellow";
                level = "warn: ";
                break;
            case LogLevel.Error:
                level = "error: ";
                color = "red";
                break;
            case LogLevel.Critical:
                level = "critical: ";
                color = "red";
                break;
        }

        // Write the log level and message
        if (color != null)
        {
            textWriter.Write(WrapColor(level, color));
        }
        else
        {
            textWriter.Write(level);
        }


        textWriter.WriteLine(logEntry.Formatter(logEntry.State, logEntry.Exception));


    }
    public static string WrapColor(string message, string color)
    {
        // Determine the ANSI color code based on the input string
        string ansiColorCode = color.ToLower() switch
        {
            "black" => "\u001b[30m",
            "red" => "\u001b[31m",
            "green" => "\u001b[32m",
            "yellow" => "\u001b[33m",
            "blue" => "\u001b[34m",
            "magenta" => "\u001b[35m",
            "cyan" => "\u001b[36m",
            "white" => "\u001b[37m",
            _ => "\u001b[37m" // Default to white if the color is unrecognized
        };

        // Wrap the message with the ANSI color code
        string coloredMessage = $"{ansiColorCode}{message}\u001b[0m";

        return coloredMessage;
    }

}
