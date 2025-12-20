use proxy_wasm::traits::{Context, HttpContext};
use proxy_wasm::types::{Action, LogLevel};

use prost::Message;
pub mod kv {
    include!(concat!(env!("OUT_DIR"), "/kv.rs"));
}


#[no_mangle]
pub fn _start() {
    proxy_wasm::set_log_level(LogLevel::Trace);
    proxy_wasm::set_http_context(|context_id, _| -> Box<dyn HttpContext> {
        Box::new(Buffer { context_id })
    });
}

struct Buffer {
    #[allow(unused)]
    context_id: u32,
}

impl Context for Buffer {}

impl HttpContext for Buffer {
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

        // Replace the message body if it contains the text "secret".
        // Since we returned "Pause" previuously, this will return the whole body.
        if let Some(body) = self.get_http_request_body(0, body_size) {
            // log::warn!("body: {:?}", body);
            // Parse grpc payload, skip the first 5 bytes
            match kv::SetRequest::decode(&body[5..]) {
                Ok(req) => {
                    // log::info!("req: {:?}", req);
                    log::warn!("Requestvalue.len(): {}", req.value.len());
                }
                Err(e) => log::warn!("decode error: {}", e),
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
        log::warn!("executing on_http_response_body, body_size: {}, end_of_stream: {}", _body_size, end_of_stream);

        // Try to get the full body even if end_of_stream is false
        // Use a large size to get all available buffered data
        let max_size = if end_of_stream { _body_size } else { usize::MAX };
        
        if let Some(body) = self.get_http_response_body(0, max_size) {
            log::warn!("got response body, body.len(): {}, end_of_stream: {}", body.len(), end_of_stream);
            if body.len() < 5 {
                log::warn!("body too short ({} bytes), need at least 5 bytes for gRPC header", body.len());
                if !end_of_stream {
                    return Action::Pause;
                }
                return Action::Continue;
            }
            // log::warn!("body: {:?}", body);
            // Parse grpc payload, skip the first 5 bytes
            match kv::GetResponse::decode(&body[5..]) {
                Ok(req) => {
                    // log::info!("req: {:?}", req);
                    log::warn!("Response value.len(): {}", req.value.len());
                    // log::warn!("body : {}", req.value);
                }
                Err(e) => log::warn!("decode error: {}", e),
            }
        } else {
            log::warn!("get_http_response_body returned None");
            if !end_of_stream {
                return Action::Pause;
            }
        }

        // If we haven't seen the end of stream yet, pause to wait for more data
        if !end_of_stream {
            log::warn!("end_of_stream is false, pausing to wait for more data");
            return Action::Pause;
        }

        Action::Continue
    }
}
