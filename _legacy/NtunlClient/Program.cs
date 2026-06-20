using Microsoft.Extensions.Logging.Console;
using NtunlClient.Services;
using NtunlCommon;

// Add any other namespaces required for your services

public class Program
{
    public static async Task Main(string[] args)
    {


        var host = Host.CreateDefaultBuilder(args)
            .ConfigureServices((hostContext, services) =>
            {
                // Configuration
                IConfiguration configuration = hostContext.Configuration;

                // Services

                services.AddSingleton(() => configuration);
                services.AddSingleton<HttpLogger>();
                services.AddSingleton<ClientMessageHandler>();

                services.AddHttpClient();


                services.AddLogging(logging =>
                {
                    logging.AddConsole(options =>
                    {
                        options.FormatterName = nameof(LogFormatter);
                    });

                    logging.AddConsoleFormatter<LogFormatter, ConsoleFormatterOptions>();
                });
                services.AddSingleton<TunnelWorker>();
                // Add hosted service
                services.AddHostedService((p) => p.GetRequiredService<TunnelWorker>());
                services.AddHostedService<HttpServer>();

            })
            .ConfigureAppConfiguration((hostingContext, config) =>
            {

                // Additional configuration settings can be set here if needed
                config.AddJsonFile("appsettings.json", optional: true, reloadOnChange: true).AddEnvironmentVariables().AddUserSecrets<Program>();

            })
            .Build();

        await host.RunAsync();
    }
}

