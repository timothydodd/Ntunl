
# NTunl
NTunl is a lightweight and flexible tunneling solution, designed to expose your local services to the internet securely. Similar to tools like ngrok, NTunl allows you to securely tunnel your localhost or any other private services to a public domain.

## Features
- Secure Tunneling: Create secure tunnels to expose your services.
- Flexible Configuration: Both server and client components are highly configurable.
- SSL Support: Secure your tunnels with SSL, including options for invalid certificates.
- Domain and Subdomain Management: Easily manage custom domains and subdomains.
- Request Inspection: Enable the Inspector to view and monitor HTTP requests through a web interface.

## Architecture
NTunl consists of two main components:

- NTunl Server: Manages and exposes the tunnels, allowing clients to connect to the public interface.
- NTunl Client: Connects to the NTunl Server to expose local or private services to the public internet.

## Getting Started
Prerequisites
- .NET 8 SDK installed on your machine.

1. Installation
Clone the repository:
``` bash
git clone https://github.com/timothydodd/ntunl.git
cd ntunl

```
2. Build the solution:
``` bash
dotnet build
```

3. Run the server

``` bash
cd src/NTunlServer
dotnet run
```

4. Run the client:

``` bash

cd src/NTunlClient
dotnet run

```


## Configuration
NTunl Server
The server exposes your tunnels to a public interface. Below is the configuration structure:
``` json
{
    "Logging": {
        "LogLevel": {
            "Default": "Information"
        }
    },
    "AllowedHosts": "*",
    "TunnelHost": {
        "HostName": "*",
        "Port": 8001,
        "ClientDomain": {
            "Domain": "dodd.rocks",
            "SubDomains": [ "apple", "banana", "cherry", "elderberry" ]
        },
        "Ssl": {
            "Enabled": false,
            "AcceptInvalidCertificates": true,
            "MutuallyAuthenticate": false,
            "Certificate": {
                "Path": "server.pfx",
                "Password": ""
            }
        }
    },
    "HttpHost": {
        "HostName": "*",
        "Port": 9200,
        "Headers": {
            "BlackList": [ "cf-*" ],
            "IpHeaderName": "X-Forwarded-For"
        }
    }
}
```
- TunnelHost: Configuration for the tunnel server, including hostnames, ports, and SSL settings.
- HttpHost: Manages HTTP connections, including header management.


## NTunl Client
The client connects to the NTunl server to expose your local service. Below is the configuration structure:

``` json
{
    "Logging": {
        "LogLevel": {
            "Default": "Debug",
            "Microsoft": "Warning",
            "Microsoft.Hosting.Lifetime": "Information",
            "System.Net.Http.HttpClient": "Warning"
        }
    },
    "Tunnels": [
        {
            "SslEnabled": true,
            "AllowInvalidCertificates": false,
            "NtunlAddress": "tunnel.mysite.com:443",
            "Address": "https://robododd.com",
            "HostHeader": "robododd.com",
            "CustomHeader": [],
            "RewriteUrlEnabled": false,
            "RewriteUrlPattern": "https://(mysite|www.mysite2)\\.com"
        }
    ],
    "Inspector": {
        "Enabled": true,
        "Port": 6900
    }
}
```
- Tunnels: Configure each tunnel with SSL settings, target addresses, and custom headers.
- Inspector: Enable to view HTTP requests through a web interface on the specified port.



# Inspecting Requests
If the Inspector is enabled in the client configuration, you can access the web interface to view HTTP requests:

Open your browser and navigate to http://localhost:6900.
# License
This project is licensed under the MIT License. See the LICENSE file for details.

# Contributing
Contributions are welcome! Please feel free to submit a Pull Request.
