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
extern crate ordered_float;
extern crate thrift;
extern crate thrift_test; // huh. I have to do this to use my lib

use ordered_float::OrderedFloat;
use std::collections::{BTreeMap, BTreeSet};
use std::fmt::Debug;

use thrift::protocol::{TBinaryInputProtocol, TBinaryOutputProtocol, TCompactInputProtocol,
                       TCompactOutputProtocol, TInputProtocol, TOutputProtocol};
use thrift::transport::{ReadHalf, TBufferedReadTransport, TBufferedWriteTransport,
                        TFramedReadTransport, TFramedWriteTransport, TIoChannel, TReadTransport,
                        TTcpChannel, TWriteTransport, WriteHalf};
use thrift_test::*;

fn main() {
    match run() {
        Ok(()) => println!("cross-test client succeeded"),
        Err(e) => {
            println!("cross-test client failed with error {:?}", e);
            std::process::exit(1);
        }
    }
}

fn run() -> thrift::Result<()> {
    // unsupported options:
    // --domain-socket
    // --named-pipe
    // --anon-pipes
    // --ssl
    // --threads
    let matches = clap_app!(rust_test_client =>
        (version: "1.0")
        (author: "Apache Thrift Developers <dev@thrift.apache.org>")
        (about: "Rust Thrift test client")
        (@arg host: --host +takes_value "Host on which the Thrift test server is located")
        (@arg port: --port +takes_value "Port on which the Thrift test server is listening")
        (@arg transport: --transport +takes_value "Thrift transport implementation to use (\"buffered\", \"framed\")")
        (@arg protocol: --protocol +takes_value "Thrift protocol implementation to use (\"binary\", \"compact\")")
        (@arg testloops: -n --testloops +takes_value "Number of times to run tests")
    )
            .get_matches();

    let host = matches.value_of("host").unwrap_or("127.0.0.1");
    let port = value_t!(matches, "port", u16).unwrap_or(9090);
    let testloops = value_t!(matches, "testloops", u8).unwrap_or(1);
    let transport = matches.value_of("transport").unwrap_or("buffered");
    let protocol = matches.value_of("protocol").unwrap_or("binary");

    let (i_chan, o_chan) = tcp_channel(host, port)?;

    let (i_tran, o_tran) = match transport {
        "buffered" => {
            (Box::new(TBufferedReadTransport::new(i_chan)) as Box<TReadTransport>,
             Box::new(TBufferedWriteTransport::new(o_chan)) as Box<TWriteTransport>)
        }
        "framed" => {
            (Box::new(TFramedReadTransport::new(i_chan)) as Box<TReadTransport>,
             Box::new(TFramedWriteTransport::new(o_chan)) as Box<TWriteTransport>)
        }
        unmatched => return Err(format!("unsupported transport {}", unmatched).into()),
    };

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

    println!(
        "connecting to {}:{} with {}+{} stack",
        host,
        port,
        protocol,
        transport
    );

    let mut client = ThriftTestSyncClient::new(i_prot, o_prot);

    for _ in 0..testloops {
        make_thrift_calls(&mut client)?
    }

    Ok(())
}

// FIXME: expose "open" through the client interface so I don't have to early
// open
fn tcp_channel(
    host: &str,
    port: u16,
) -> thrift::Result<(ReadHalf<TTcpChannel>, WriteHalf<TTcpChannel>)> {
    let mut c = TTcpChannel::new();
    c.open(&format!("{}:{}", host, port))?;
    c.split()
}

fn make_thrift_calls(client: &mut ThriftTestSyncClient<Box<TInputProtocol>, Box<TOutputProtocol>>,)
    -> Result<(), thrift::Error> {
    println!("testVoid");
    client.test_void()?;

    println!("testString");
    verify_expected_result(client.test_string("thing".to_owned()), "thing".to_owned())?;

    println!("testBool");
    verify_expected_result(client.test_bool(true), true)?;

    println!("testBool");
    verify_expected_result(client.test_bool(false), false)?;

    println!("testByte");
    verify_expected_result(client.test_byte(42), 42)?;

    println!("testi32");
    verify_expected_result(client.test_i32(1159348374), 1159348374)?;

    println!("testi64");
    // try!(verify_expected_result(client.test_i64(-8651829879438294565),
    // -8651829879438294565));
    verify_expected_result(client.test_i64(i64::min_value()), i64::min_value())?;

    println!("testDouble");
    verify_expected_result(
        client.test_double(OrderedFloat::from(42.42)),
        OrderedFloat::from(42.42),
    )?;

    println!("testTypedef");
    {
        let u_snd: UserId = 2348;
        let u_cmp: UserId = 2348;
        verify_expected_result(client.test_typedef(u_snd), u_cmp)?;
    }

    println!("testEnum");
    {
        verify_expected_result(client.test_enum(Numberz::TWO), Numberz::TWO)?;
    }

    println!("testBinary");
    {
        let b_snd = vec![0x77, 0x30, 0x30, 0x74, 0x21, 0x20, 0x52, 0x75, 0x73, 0x74];
        let b_cmp = vec![0x77, 0x30, 0x30, 0x74, 0x21, 0x20, 0x52, 0x75, 0x73, 0x74];
        verify_expected_result(client.test_binary(b_snd), b_cmp)?;
    }

    println!("testStruct");
    {
        let x_snd = Xtruct {
            string_thing: Some("foo".to_owned()),
            byte_thing: Some(12),
            i32_thing: Some(219129),
            i64_thing: Some(12938492818),
        };
        let x_cmp = Xtruct {
            string_thing: Some("foo".to_owned()),
            byte_thing: Some(12),
            i32_thing: Some(219129),
            i64_thing: Some(12938492818),
        };
        verify_expected_result(client.test_struct(x_snd), x_cmp)?;
    }

    // Xtruct again, with optional values
    // FIXME: apparently the erlang thrift server does not like opt-in-req-out
    // parameters that are undefined. Joy.
    // {
    // let x_snd = Xtruct { string_thing: Some("foo".to_owned()), byte_thing: None,
    // i32_thing: None, i64_thing: Some(12938492818) };
    // let x_cmp = Xtruct { string_thing: Some("foo".to_owned()), byte_thing:
    // Some(0), i32_thing: Some(0), i64_thing: Some(12938492818) }; // the C++
    // server is responding correctly
    // try!(verify_expected_result(client.test_struct(x_snd), x_cmp));
    // }
    //


    println!("testNest"); // (FIXME: try Xtruct2 with optional values)
    {
        let x_snd = Xtruct2 {
            byte_thing: Some(32),
            struct_thing: Some(
                Xtruct {
                    string_thing: Some("foo".to_owned()),
                    byte_thing: Some(1),
                    i32_thing: Some(324382098),
                    i64_thing: Some(12938492818),
                },
            ),
            i32_thing: Some(293481098),
        };
        let x_cmp = Xtruct2 {
            byte_thing: Some(32),
            struct_thing: Some(
                Xtruct {
                    string_thing: Some("foo".to_owned()),
                    byte_thing: Some(1),
                    i32_thing: Some(324382098),
                    i64_thing: Some(12938492818),
                },
            ),
            i32_thing: Some(293481098),
        };
        verify_expected_result(client.test_nest(x_snd), x_cmp)?;
    }

    println!("testList");
    {
        let mut v_snd: Vec<i32> = Vec::new();
        v_snd.push(29384);
        v_snd.push(238);
        v_snd.push(32498);

        let mut v_cmp: Vec<i32> = Vec::new();
        v_cmp.push(29384);
        v_cmp.push(238);
        v_cmp.push(32498);

        verify_expected_result(client.test_list(v_snd), v_cmp)?;
    }

    println!("testSet");
    {
        let mut s_snd: BTreeSet<i32> = BTreeSet::new();
        s_snd.insert(293481);
        s_snd.insert(23);
        s_snd.insert(3234);

        let mut s_cmp: BTreeSet<i32> = BTreeSet::new();
        s_cmp.insert(293481);
        s_cmp.insert(23);
        s_cmp.insert(3234);

        verify_expected_result(client.test_set(s_snd), s_cmp)?;
    }

    println!("testMap");
    {
        let mut m_snd: BTreeMap<i32, i32> = BTreeMap::new();
        m_snd.insert(2, 4);
        m_snd.insert(4, 6);
        m_snd.insert(8, 7);

        let mut m_cmp: BTreeMap<i32, i32> = BTreeMap::new();
        m_cmp.insert(2, 4);
        m_cmp.insert(4, 6);
        m_cmp.insert(8, 7);

        verify_expected_result(client.test_map(m_snd), m_cmp)?;
    }

    println!("testStringMap");
    {
        let mut m_snd: BTreeMap<String, String> = BTreeMap::new();
        m_snd.insert("2".to_owned(), "4_string".to_owned());
        m_snd.insert("4".to_owned(), "6_string".to_owned());
        m_snd.insert("8".to_owned(), "7_string".to_owned());

        let mut m_rcv: BTreeMap<String, String> = BTreeMap::new();
        m_rcv.insert("2".to_owned(), "4_string".to_owned());
        m_rcv.insert("4".to_owned(), "6_string".to_owned());
        m_rcv.insert("8".to_owned(), "7_string".to_owned());

        verify_expected_result(client.test_string_map(m_snd), m_rcv)?;
    }

    // nested map
    // expect : {-4 => {-4 => -4, -3 => -3, -2 => -2, -1 => -1, }, 4 => {1 => 1, 2
    // => 2, 3 => 3, 4 => 4, }, }
    println!("testMapMap");
    {
        let mut m_cmp_nested_0: BTreeMap<i32, i32> = BTreeMap::new();
        for i in (-4 as i32)..0 {
            m_cmp_nested_0.insert(i, i);
        }
        let mut m_cmp_nested_1: BTreeMap<i32, i32> = BTreeMap::new();
        for i in 1..5 {
            m_cmp_nested_1.insert(i, i);
        }

        let mut m_cmp: BTreeMap<i32, BTreeMap<i32, i32>> = BTreeMap::new();
        m_cmp.insert(-4, m_cmp_nested_0);
        m_cmp.insert(4, m_cmp_nested_1);

        verify_expected_result(client.test_map_map(42), m_cmp)?;
    }

    println!("testMulti");
    {
        let mut m_snd: BTreeMap<i16, String> = BTreeMap::new();
        m_snd.insert(1298, "fizz".to_owned());
        m_snd.insert(-148, "buzz".to_owned());

        let s_cmp = Xtruct {
            string_thing: Some("Hello2".to_owned()),
            byte_thing: Some(1),
            i32_thing: Some(-123948),
            i64_thing: Some(-19234123981),
        };

        verify_expected_result(
            client.test_multi(1, -123948, -19234123981, m_snd, Numberz::EIGHT, 81),
            s_cmp,
        )?;
    }

    // Insanity
    // returns:
    // { 1 => { 2 => argument,
    //          3 => argument,
    //        },
    //   2 => { 6 => <empty Insanity struct>, },
    // }
    {
        let mut arg_map_usermap: BTreeMap<Numberz, i64> = BTreeMap::new();
        arg_map_usermap.insert(Numberz::ONE, 4289);
        arg_map_usermap.insert(Numberz::EIGHT, 19);

        let mut arg_vec_xtructs: Vec<Xtruct> = Vec::new();
        arg_vec_xtructs.push(
            Xtruct {
                string_thing: Some("foo".to_owned()),
                byte_thing: Some(8),
                i32_thing: Some(29),
                i64_thing: Some(92384),
            },
        );
        arg_vec_xtructs.push(
            Xtruct {
                string_thing: Some("bar".to_owned()),
                byte_thing: Some(28),
                i32_thing: Some(2),
                i64_thing: Some(-1281),
            },
        );
        arg_vec_xtructs.push(
            Xtruct {
                string_thing: Some("baz".to_owned()),
                byte_thing: Some(0),
                i32_thing: Some(3948539),
                i64_thing: Some(-12938492),
            },
        );

        let mut s_cmp_nested_1: BTreeMap<Numberz, Insanity> = BTreeMap::new();
        let insanity = Insanity {
            user_map: Some(arg_map_usermap),
            xtructs: Some(arg_vec_xtructs),
        };
        s_cmp_nested_1.insert(Numberz::TWO, insanity.clone());
        s_cmp_nested_1.insert(Numberz::THREE, insanity.clone());

        let mut s_cmp_nested_2: BTreeMap<Numberz, Insanity> = BTreeMap::new();
        let empty_insanity = Insanity {
            user_map: Some(BTreeMap::new()),
            xtructs: Some(Vec::new()),
        };
        s_cmp_nested_2.insert(Numberz::SIX, empty_insanity);

        let mut s_cmp: BTreeMap<UserId, BTreeMap<Numberz, Insanity>> = BTreeMap::new();
        s_cmp.insert(1 as UserId, s_cmp_nested_1);
        s_cmp.insert(2 as UserId, s_cmp_nested_2);

        verify_expected_result(client.test_insanity(insanity.clone()), s_cmp)?;
    }

    println!("testException - remote throws Xception");
    {
        let r = client.test_exception("Xception".to_owned());
        let x = match r {
            Err(thrift::Error::User(ref e)) => {
                match e.downcast_ref::<Xception>() {
                    Some(x) => Ok(x),
                    None => Err(thrift::Error::User("did not get expected Xception struct".into()),),
                }
            }
            _ => Err(thrift::Error::User("did not get exception".into())),
        }?;

        let x_cmp = Xception {
            error_code: Some(1001),
            message: Some("Xception".to_owned()),
        };

        verify_expected_result(Ok(x), &x_cmp)?;
    }

    println!("testException - remote throws TApplicationException");
    {
        let r = client.test_exception("TException".to_owned());
        match r {
            Err(thrift::Error::Application(ref e)) => {
                println!("received an {:?}", e);
                Ok(())
            }
            _ => Err(thrift::Error::User("did not get exception".into())),
        }?;
    }

    println!("testException - remote succeeds");
    {
        let r = client.test_exception("foo".to_owned());
        match r {
            Ok(_) => Ok(()),
            _ => Err(thrift::Error::User("received an exception".into())),
        }?;
    }

    println!("testMultiException - remote throws Xception");
    {
        let r = client.test_multi_exception("Xception".to_owned(), "ignored".to_owned());
        let x = match r {
            Err(thrift::Error::User(ref e)) => {
                match e.downcast_ref::<Xception>() {
                    Some(x) => Ok(x),
                    None => Err(thrift::Error::User("did not get expected Xception struct".into()),),
                }
            }
            _ => Err(thrift::Error::User("did not get exception".into())),
        }?;

        let x_cmp = Xception {
            error_code: Some(1001),
            message: Some("This is an Xception".to_owned()),
        };

        verify_expected_result(Ok(x), &x_cmp)?;
    }

    println!("testMultiException - remote throws Xception2");
    {
        let r = client.test_multi_exception("Xception2".to_owned(), "ignored".to_owned());
        let x = match r {
            Err(thrift::Error::User(ref e)) => {
                match e.downcast_ref::<Xception2>() {
                    Some(x) => Ok(x),
                    None => Err(thrift::Error::User("did not get expected Xception struct".into()),),
                }
            }
            _ => Err(thrift::Error::User("did not get exception".into())),
        }?;

        let x_cmp = Xception2 {
            error_code: Some(2002),
            struct_thing: Some(
                Xtruct {
                    string_thing: Some("This is an Xception2".to_owned()),
                    // since this is an OPT_IN_REQ_OUT field the sender sets a default
                    byte_thing: Some(0),
                    // since this is an OPT_IN_REQ_OUT field the sender sets a default
                    i32_thing: Some(0),
                    // since this is an OPT_IN_REQ_OUT field the sender sets a default
                    i64_thing: Some(0),
                },
            ),
        };

        verify_expected_result(Ok(x), &x_cmp)?;
    }

    println!("testMultiException - remote succeeds");
    {
        let r = client.test_multi_exception("haha".to_owned(), "RETURNED".to_owned());
        let x = match r {
            Err(e) => Err(thrift::Error::User(format!("received an unexpected exception {:?}", e).into(),),),
            _ => r,
        }?;

        let x_cmp = Xtruct {
            string_thing: Some("RETURNED".to_owned()),
            // since this is an OPT_IN_REQ_OUT field the sender sets a default
            byte_thing: Some(0),
            // since this is an OPT_IN_REQ_OUT field the sender sets a default
            i32_thing: Some(0),
            // since this is an OPT_IN_REQ_OUT field the sender sets a default
            i64_thing: Some(0),
        };

        verify_expected_result(Ok(x), x_cmp)?;
    }

    println!("testOneWay - remote sleeps for 1 second");
    {
        client.test_oneway(1)?;
    }

    // final test to verify that the connection is still writable after the one-way
    // call
    client.test_void()
}

#[cfg_attr(feature = "cargo-clippy", allow(needless_pass_by_value))]
fn verify_expected_result<T: Debug + PartialEq + Sized>(
    actual: Result<T, thrift::Error>,
    expected: T,
) -> Result<(), thrift::Error> {
    match actual {
        Ok(v) => {
            if v == expected {
                Ok(())
            } else {
                Err(thrift::Error::User(format!("expected {:?} but got {:?}", &expected, &v).into()),)
            }
        }
        Err(e) => Err(e),
    }
}
