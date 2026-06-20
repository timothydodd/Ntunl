using Microsoft.Extensions.Logging.Console;
using NtunlCommon;
using NtunlHost.Services;

namespace NtunlHost;

public class Program
{
    public static async Task Main(string[] args)
    {
        var host = Host.CreateDefaultBuilder(args)
            .ConfigureServices((hostContext, services) =>
            {
                IConfiguration configuration = hostContext.Configuration;


                services.Configure<TunnelHostSettings>(configuration.GetRequiredSection("TunnelHost"));
                services.Configure<HttpHostSetting>(configuration.GetRequiredSection("HttpHost"));
                services.AddSingleton<HttpServerMessageHandler>();
                services.AddSingleton<TunnelHostWorker>();
                services.AddHostedService(provider => provider.GetRequiredService<TunnelHostWorker>());
                services.AddHostedService<HttpServer>();
                services.AddSingleton(() => configuration);
                services.AddSingleton<TunnelHost>();
                services.AddLogging(logging =>
                {
                    logging.AddConsole(options =>
                    {
                        options.FormatterName = nameof(LogFormatter);
                    });

                    logging.AddConsoleFormatter<LogFormatter, ConsoleFormatterOptions>();
                });


            })
            .ConfigureAppConfiguration((hostingContext, config) =>
            {
                // Additional configuration settings can be set here if needed
                config.AddJsonFile("appsettings.json", optional: true, reloadOnChange: true).AddEnvironmentVariables();

            })
            .Build();

        await host.RunAsync();
    }

}
