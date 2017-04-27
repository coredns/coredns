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

use std::collections::HashMap;
use std::convert::Into;
use std::sync::{Arc, Mutex};

use {ApplicationErrorKind, new_application_error};
use protocol::{TInputProtocol, TMessageIdentifier, TOutputProtocol, TStoredInputProtocol};

use super::TProcessor;

/// A `TProcessor` that can demux service calls to multiple underlying
/// Thrift services.
///
/// Users register service-specific `TProcessor` instances with a
/// `TMultiplexedProcessor`, and then register that processor with a server
/// implementation. Following that, all incoming service calls are automatically
/// routed to the service-specific `TProcessor`.
///
/// A `TMultiplexedProcessor` can only handle messages sent by a
/// `TMultiplexedOutputProtocol`.
// FIXME: implement Debug
pub struct TMultiplexedProcessor {
    processors: Mutex<HashMap<String, Arc<Box<TProcessor>>>>,
}

impl TMultiplexedProcessor {
    /// Register a service-specific `processor` for the service named
    /// `service_name`.
    ///
    /// Return `true` if this is the first registration for `service_name`.
    ///
    /// Return `false` if a mapping previously existed (the previous mapping is
    /// *not* overwritten).
    #[cfg_attr(feature = "cargo-clippy", allow(map_entry))]
    pub fn register_processor<S: Into<String>>(
        &mut self,
        service_name: S,
        processor: Box<TProcessor>,
    ) -> bool {
        let mut processors = self.processors.lock().unwrap();

        let name = service_name.into();
        if processors.contains_key(&name) {
            false
        } else {
            processors.insert(name, Arc::new(processor));
            true
        }
    }
}

impl TProcessor for TMultiplexedProcessor {
    fn process(&self, i_prot: &mut TInputProtocol, o_prot: &mut TOutputProtocol) -> ::Result<()> {
        let msg_ident = i_prot.read_message_begin()?;
        let sep_index = msg_ident
            .name
            .find(':')
            .ok_or_else(
                || {
                    new_application_error(
                        ApplicationErrorKind::Unknown,
                        "no service separator found in incoming message",
                    )
                },
            )?;

        let (svc_name, svc_call) = msg_ident.name.split_at(sep_index);

        let processor: Option<Arc<Box<TProcessor>>> = {
            let processors = self.processors.lock().unwrap();
            processors.get(svc_name).cloned()
        };

        match processor {
            Some(arc) => {
                let new_msg_ident = TMessageIdentifier::new(
                    svc_call,
                    msg_ident.message_type,
                    msg_ident.sequence_number,
                );
                let mut proxy_i_prot = TStoredInputProtocol::new(i_prot, new_msg_ident);
                (*arc).process(&mut proxy_i_prot, o_prot)
            }
            None => {
                Err(
                    new_application_error(
                        ApplicationErrorKind::Unknown,
                        format!("no processor found for service {}", svc_name),
                    ),
                )
            }
        }
    }
}
