namespace go kv

service KVService {
    GetResponse get(1: GetRequest req)
    SetResponse setValue(1: SetRequest req)
}

struct GetRequest {
    1: string key
}

struct GetResponse {
    1: string value
}

struct SetRequest {
    1: string key
    2: string value
}

struct SetResponse {
    1: string value
}

