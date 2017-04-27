// Licensed to the Apache Software Foundation (ASF) under one
// or more contributor license agreements. See the NOTICE file
// distributed with this work for additional information
// regarding copyright ownership. The ASF licenses this file
// to you under the Apache License, Version 2.0 (the
// "License"); you may not use this file except in compliance
// with the License. You may obtain a copy of the License at
//
//   http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied. See the License for the
// specific language governing permissions and limitations
// under the License.

#[macro_use]
extern crate clap;

extern crate kitchen_sink;
extern crate thrift;

use kitchen_sink::base_two::{TNapkinServiceSyncClient, TRamenServiceSyncClient};
use kitchen_sink::midlayer::{MealServiceSyncClient, TMealServiceSyncClient};
use kitchen_sink::ultimate::{FullMealServiceSyncClient, TFullMealServiceSyncClient};
use thrift::transport::{ReadHalf, TFramedReadTransport, TFramedWriteTransport, TIoChannel,
                        TTcpChannel, WriteHalf};
use thrift::protocol::{TBinaryInputProtocol, TBinaryOutputProtocol, TCompactInputProtocol,
                       TCompactOutputProtocol, TInputProtocol, TOutputProtocol};

fn main() {
    match run() {
        Ok(()) => println!("kitchen sink client completed successfully"),
        Err(e) => {
            println!("kitchen sink client failed with error {:?}", e);
            std::process::exit(1);
        }
    }
}

fn run() -> thrift::Result<()> {
    let matches = clap_app!(rust_kitchen_sink_client =>
        (version: "0.1.0")
        (author: "Apache Thrift Developers <dev@thrift.apache.org>")
        (about: "Thrift Rust kitchen sink client")
        (@arg host: --host +takes_value "Host on which the Thrift test server is located")
        (@arg port: --port +takes_value "Port on which the Thrift test server is listening")
        (@arg protocol: --protocol +takes_value "Thrift protocol implementation to use (\"binary\", \"compact\")")
        (@arg service: --service +takes_value "Service type to contact (\"part\", \"full\")")
    )
            .get_matches();

    let host = matches.value_of("host").unwrap_or("127.0.0.1");
    let port = value_t!(matches, "port", u16).unwrap_or(9090);
    let protocol = matches.value_of("protocol").unwrap_or("compact");
    let service = matches.value_of("service").unwrap_or("part");

    let (i_chan, o_chan) = tcp_channel(host, port)?;
    let (i_tran, o_tran) = (TFramedReadTransport::new(i_chan), TFramedWriteTransport::new(o_chan));

    let (i_prot, o_prot): (Box<TInputProtocol>, Box<TOutputProtocol>) = match protocol {
        "binary" => {
            (Box::new(TBinaryInputProtocol::new(i_tran, true)),
             Box::new(TBinaryOutputProtocol::new(o_tran, true)))
        }
        "compact" => {
            (Box::new(TCompactInputProtocol::new(i_tran)),
             Box::new(TCompactOutputProtocol::new(o_tran)))
        }
        unmatched => return Err(format!("unsupported protocol {}", unmatched).into()),
    };

    run_client(service, i_prot, o_prot)
}

fn run_client(
    service: &str,
    i_prot: Box<TInputProtocol>,
    o_prot: Box<TOutputProtocol>,
) -> thrift::Result<()> {
    match service {
        "full" => run_full_meal_service(i_prot, o_prot),
        "part" => run_meal_service(i_prot, o_prot),
        _ => Err(thrift::Error::from(format!("unknown service type {}", service)),),
    }
}

fn tcp_channel(
    host: &str,
    port: u16,
) -> thrift::Result<(ReadHalf<TTcpChannel>, WriteHalf<TTcpChannel>)> {
    let mut c = TTcpChannel::new();
    c.open(&format!("{}:{}", host, port))?;
    c.split()
}

fn run_meal_service(
    i_prot: Box<TInputProtocol>,
    o_prot: Box<TOutputProtocol>,
) -> thrift::Result<()> {
    let mut client = MealServiceSyncClient::new(i_prot, o_prot);

    // client.full_meal(); // <-- IMPORTANT: if you uncomment this, compilation *should* fail
    // this is because the MealService struct does not contain the appropriate service marker

    // only the following three calls work
    execute_call("part", "ramen", || client.ramen(50))?;
    execute_call("part", "meal", || client.meal())?;
    execute_call("part", "napkin", || client.napkin())?;

    Ok(())
}

fn run_full_meal_service(
    i_prot: Box<TInputProtocol>,
    o_prot: Box<TOutputProtocol>,
) -> thrift::Result<()> {
    let mut client = FullMealServiceSyncClient::new(i_prot, o_prot);

    execute_call("full", "ramen", || client.ramen(100))?;
    execute_call("full", "meal", || client.meal())?;
    execute_call("full", "napkin", || client.napkin())?;
    execute_call("full", "full meal", || client.full_meal())?;

    Ok(())
}

fn execute_call<F, R>(service_type: &str, call_name: &str, mut f: F) -> thrift::Result<()>
where
    F: FnMut() -> thrift::Result<R>,
{
    let res = f();

    match res {
        Ok(_) => println!("{}: completed {} call", service_type, call_name),
        Err(ref e) => {
            println!(
                "{}: failed {} call with error {:?}",
                service_type,
                call_name,
                e
            )
        }
    }

    res.map(|_| ())
}
