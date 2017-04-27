# Apache Thrift net-core-lib

Thrift client library ported to Microsoft .Net Core 

# Content
- Tests/Thrift.PublicInterfaces.Compile.Tests - project for checking public interfaces during adding changes to Thrift library
- Thrift - Thrift library 

# Reused components 
- NET Core Standard 1.6 (SDK 1.0.0-preview2-003121)
- NET Core App 1.1

# How to build

- Download and install .NET Core SDK for your platform https://www.microsoft.com/net/core#windowsvs2015
- Ensure that you have thrift.exe which supports netcore lib and it added to PATH 
- Go to current folder 
- Run **build.sh** or **build.cmd** from the root of cloned repository
- Check tests in **src/Tests** folder
- Continue with /tutorials/netcore 

#Notes

- Migration to .NET Standard 2.0 planned for later (Q1 2017) according to  https://blogs.msdn.microsoft.com/dotnet/2016/09/26/introducing-net-standard/
- Possible adding additional platforms after stabilization of .NET Core (runtimes, platforms (Red Haat Linux, OpenSuse, etc.) 

#Known issues

- In trace logging mode you can see some not important internal exceptions
- Ubuntu 16.10 still not supported fully 
- There is some problems with .NET Core CLI and usage specific -r|--runtime for building and publishing projects with different target frameworks (netstandard1.6 and netcoreapp1.1) 

# Troubleshouting 

It's possible to change dotnet SDK version for building for solution (in **global.json**). Just run **dotnet --info** to check your current version (or check your dotnet sdk folder for installed versions)