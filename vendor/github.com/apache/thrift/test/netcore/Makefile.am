#
# Licensed to the Apache Software Foundation (ASF) under one
# or more contributor license agreements. See the NOTICE file
# distributed with this work for additional information
# regarding copyright ownership. The ASF licenses this file
# to you under the Apache License, Version 2.0 (the
# "License"); you may not use this file except in compliance
# with the License. You may obtain a copy of the License at
#
#   http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied. See the License for the
# specific language governing permissions and limitations
# under the License.
#

SUBDIRS = . 

THRIFT = $(top_builddir)/compiler/cpp/thrift

GENDIR = ThriftTest/gen-netcore

# Due to a known issue with "dotnet restore" the Thrift.dll dependency cannot be resolved from cmdline.
# The problem does NOT affect Visual Studio builds, only cmdline. 
# - For details see https://github.com/dotnet/cli/issues/3199 and related tickets.
# - Workaround is to temporarily copy the Thrift project into the solution
COPYCMD = cp  -u -p -r
	

THRIFTCODE = \
			ThriftTest/TestClient.cs \
			ThriftTest/TestServer.cs \
			ThriftTest/Properties/AssemblyInfo.cs \
			ThriftTest/Program.cs 

all-local: \
	ThriftTest.exe

ThriftTest.exe: $(THRIFTCODE)
	$(MKDIR_P) $(GENDIR)
	$(THRIFT)  -gen netcore:wcf   -r  -out $(GENDIR)  $(top_srcdir)/test/ThriftTest.thrift
	$(MKDIR_P) ./Thrift
	$(COPYCMD)  $(top_srcdir)/lib/netcore/Thrift/*  ./Thrift
	$(DOTNETCORE) --info
	$(DOTNETCORE) restore
	$(DOTNETCORE) build **/*/project.json -r win10-x64 
	$(DOTNETCORE) build **/*/project.json -r osx.10.11-x64 
	$(DOTNETCORE) build **/*/project.json -r ubuntu.16.04-x64 

clean-local:
	$(RM) ThriftTest.exe
	$(RM) -r $(GENDIR)
	$(RM) -r ThriftTest/bin
	$(RM) -r ThriftTest/obj
	$(RM) -r Thrift

EXTRA_DIST = \
			 $(THRIFTCODE) \
			 global.json \
			 ThriftTest/project.json \
			 ThriftTest/ThriftTest.sln \
			 ThriftTest/ThriftTest.xproj \
			 build.cmd \
			 build.sh
			 
