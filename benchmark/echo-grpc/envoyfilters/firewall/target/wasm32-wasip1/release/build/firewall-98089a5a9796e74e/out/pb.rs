#[allow(clippy::derive_partial_eq_without_eq)]
#[derive(Clone, PartialEq, ::prost::Message)]
pub struct Msg {
    #[prost(string, tag = "1")]
    pub body: ::prost::alloc::string::String,
}
