use proxy_wasm::traits::{Context, HttpContext};
use proxy_wasm::types::{Action, LogLevel};

pub mod kv {
    include!(concat!(env!("OUT_DIR"), "/kv.rs"));
}


#[no_mangle]
pub fn _start() {
    proxy_wasm::set_log_level(LogLevel::Trace);
    proxy_wasm::set_http_context(|context_id, _| -> Box<dyn HttpContext> {
        Box::new(Stream { context_id })
    });
}

struct Stream {
    #[allow(unused)]
    context_id: u32,
}

impl Context for Stream {}

impl HttpContext for Stream {
    fn on_http_request_headers(&mut self, _num_of_headers: usize, _end_of_stream: bool) -> Action {
        log::warn!("executing on_http_request_headers");

        Action::Continue
    }

    fn on_http_request_body(&mut self, body_size: usize, end_of_stream: bool) -> Action {
        log::warn!("executing on_http_request_body, body_size: {}, end_of_stream: {}", body_size, end_of_stream);

        Action::Continue
    }

    fn on_http_response_headers(&mut self, _num_headers: usize, _end_of_stream: bool) -> Action {
        log::warn!("executing on_http_response_headers");

        Action::Continue
    }

    fn on_http_response_body(&mut self, _body_size: usize, end_of_stream: bool) -> Action {
        log::warn!("executing on_http_response_body, body_size: {}, end_of_stream: {}", _body_size, end_of_stream);

        Action::Continue
    }
}
