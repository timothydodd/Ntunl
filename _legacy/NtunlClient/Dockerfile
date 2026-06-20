#See https://aka.ms/customizecontainer to learn how to customize your debug container and how Visual Studio uses this Dockerfile to build your images for faster debugging.

FROM mcr.microsoft.com/dotnet/aspnet:8.0 AS base
USER app
WORKDIR /app

FROM mcr.microsoft.com/dotnet/sdk:9.0 AS build
ARG BUILD_CONFIGURATION=Release
WORKDIR /src
COPY ["NtunlClient/NtunlClient.csproj", "NtunlClient/"]
COPY ["NtunlCommon/NtunlCommon.csproj", "NtunlCommon/"]
RUN dotnet restore "./NtunlClient/NtunlClient.csproj"
COPY . .
WORKDIR "/src/NtunlClient"
RUN dotnet build "./NtunlClient.csproj" -c $BUILD_CONFIGURATION -o /app/build

FROM build AS publish
ARG BUILD_CONFIGURATION=Release
RUN dotnet publish "./NtunlClient.csproj" -c $BUILD_CONFIGURATION -o /app/publish /p:UseAppHost=false

FROM base AS final
WORKDIR /app
COPY --from=publish /app/publish .
ENTRYPOINT ["dotnet", "NtunlClient.dll"]
