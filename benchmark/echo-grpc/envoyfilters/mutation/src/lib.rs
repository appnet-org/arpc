use proxy_wasm::traits::{Context, HttpContext};
use proxy_wasm::types::{Action, LogLevel};
use proxy_wasm::traits::RootContext;

use prost::Message;
pub mod echo {
    include!(concat!(env!("OUT_DIR"), "/pb.rs"));
}

#[no_mangle]
pub fn _start() {
    proxy_wasm::set_log_level(LogLevel::Trace);
    proxy_wasm::set_http_context(|context_id, _| -> Box<dyn HttpContext> {
        Box::new(Mutation { context_id })
    });
}

struct Mutation {
    #[allow(unused)]
    context_id: u32,
}

impl Context for Mutation {}

impl RootContext for Mutation {
    fn on_vm_start(&mut self, _: usize) -> bool {
        log::warn!("executing on_vm_start");
        true
    }
}

impl HttpContext for Mutation {
    fn on_http_request_headers(&mut self, _num_of_headers: usize, end_of_stream: bool) -> Action {
        log::warn!("executing on_http_request_headers");
        if !end_of_stream {
            return Action::Continue;
        }

        self.set_http_response_header("content-length", None);
        Action::Continue
    }

    fn on_http_request_body(&mut self, body_size: usize, end_of_stream: bool) -> Action {
        log::warn!("executing on_http_request_body");
        if !end_of_stream {
            return Action::Pause;
        }

        if let Some(body) = self.get_http_request_body(0, body_size) {
            // log::warn!("Original body size: {}", body.len());
            if body.len() > 5 {
                // The gRPC message may be changed/compressed - better use the new length. 
                // This step is required as the body_size will be inaccurate.
                let message_length = u32::from_be_bytes([body[1], body[2], body[3], body[4]]) as usize + 5;
                log::warn!("gRPC message length: {}", message_length);
                if let Ok(mut req) = echo::EchoRequest::decode(&body[5..message_length]) {
                    // Modify the body here
                    req.message = req.message.replace("Bob", "Alice");

                    // Re-encode the modified message
                    let mut new_body = Vec::new();
                    req.encode(&mut new_body).expect("Failed to encode");

                    // log::warn!("Modified body size: {}", new_body.len());

                    // Construct the gRPC header
                    let new_body_length = new_body.len() as u32;
                    let mut grpc_header = Vec::new();
                    grpc_header.push(0); // Compression flag
                    grpc_header.extend_from_slice(&new_body_length.to_be_bytes());

                    // log::warn!("Header size: {}", grpc_header.len());

                    // Combine header and body
                    grpc_header.append(&mut new_body);

                    // Replace the request body
                    self.set_http_request_body(0, grpc_header.len(), &grpc_header);
                } else {
                    log::warn!("Failed to decode the request body");
                }
            } else {
                log::warn!("Received body is too short to be a valid gRPC message");
            }
        }

        Action::Continue
    }

    fn on_http_response_headers(&mut self, _num_headers: usize, end_of_stream: bool) -> Action {
        log::warn!("executing on_http_response_headers");
        if !end_of_stream {
            return Action::Continue;
        }

        Action::Continue
    }

    fn on_http_response_body(&mut self, _body_size: usize, end_of_stream: bool) -> Action {
        log::warn!("executing on_http_response_body");
        if !end_of_stream {
            return Action::Pause;
        }

        Action::Continue
    }
}