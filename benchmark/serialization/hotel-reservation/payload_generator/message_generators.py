"""Message-level generators for creating JSON payloads matching hotel_reservation.proto."""

from typing import Any, Callable, Dict

from field_generator import FieldGenerator


def gen_NearbyRequest(g: FieldGenerator) -> Dict[str, Any]:
    lat = g.gen_lat()
    lon = g.gen_lon()
    return {"lat": lat, "lon": lon, "latstring": g.gen_latstring(lat, lon)}


def gen_NearbyResult(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(dcfg.hotel_ids_min, dcfg.hotel_ids_max)
    return {"hotelIds": [g.gen_hotel_id() for _ in range(n)]}


def gen_GetProfilesRequest(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(dcfg.hotel_ids_min, dcfg.hotel_ids_max)
    return {"hotelIds": [g.gen_hotel_id() for _ in range(n)], "locale": g.gen_locale()}


def gen_Address(g: FieldGenerator) -> Dict[str, Any]:
    return g.gen_address()


def gen_Image(g: FieldGenerator, hotel_id: str = None, default: bool = False) -> Dict[str, Any]:
    return {"url": g.gen_image_url(hotel_id), "default": default}


def gen_Hotel(g: FieldGenerator) -> Dict[str, Any]:
    hid = g.gen_hotel_id()
    dcfg = g.cfg.dist
    n_images = g._bounded_len(dcfg.images_per_hotel_min, dcfg.images_per_hotel_max)
    images = [gen_Image(g, hid, i == 0) for i in range(n_images)]
    return {
        "id": hid,
        "name": g.gen_hotel_name(),
        "phoneNumber": g.gen_phone_number(),
        "description": g.gen_hotel_description(),
        "address": gen_Address(g),
        "images": images,
    }


def gen_GetProfilesResult(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(dcfg.hotels_per_result_min, dcfg.hotels_per_result_max)
    return {"hotels": [gen_Hotel(g) for _ in range(n)]}


def gen_GetRecommendationsRequest(g: FieldGenerator) -> Dict[str, Any]:
    req = g._choice(g.cfg.hotel_text.recommendation_requirements)
    return {"require": req, "lat": g.gen_lat(), "lon": g.gen_lon()}


def gen_GetRecommendationsResult(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(dcfg.recommendation_ids_min, dcfg.recommendation_ids_max)
    return {"HotelIds": [g.gen_hotel_id() for _ in range(n)]}


def gen_GetRatesRequest(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(dcfg.hotel_ids_min, dcfg.hotel_ids_max)
    in_d, out_d = g.gen_date_range()
    return {"hotelIds": [g.gen_hotel_id() for _ in range(n)], "inDate": in_d, "outDate": out_d}


def gen_RoomType(g: FieldGenerator) -> Dict[str, Any]:
    return g.gen_room_type()


def gen_RatePlan(g: FieldGenerator) -> Dict[str, Any]:
    in_d, out_d = g.gen_date_range()
    return {
        "hotelId": g.gen_hotel_id(),
        "code": g.gen_rate_plan_code(),
        "inDate": in_d,
        "outDate": out_d,
        "roomType": gen_RoomType(g),
    }


def gen_GetRatesResult(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(dcfg.rate_plans_min, dcfg.rate_plans_max)
    return {"ratePlans": [gen_RatePlan(g) for _ in range(n)]}


def gen_ReservationRequest(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(1, min(5, dcfg.hotel_ids_max))
    in_d, out_d = g.gen_date_range()
    return {
        "customerName": g.gen_customer_name(),
        "hotelId": [g.gen_hotel_id() for _ in range(n)],
        "inDate": in_d,
        "outDate": out_d,
        "roomNumber": g.gen_room_number(),
    }


def gen_ReservationResult(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(1, min(5, dcfg.hotel_ids_max))
    return {"hotelId": [g.gen_hotel_id() for _ in range(n)]}


def gen_SearchRequest(g: FieldGenerator) -> Dict[str, Any]:
    in_d, out_d = g.gen_date_range()
    return {"lat": g.gen_lat(), "lon": g.gen_lon(), "inDate": in_d, "outDate": out_d}


def gen_SearchResult(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(dcfg.hotel_ids_min, dcfg.hotel_ids_max)
    return {"hotelIds": [g.gen_hotel_id() for _ in range(n)]}


def gen_CheckUserRequest(g: FieldGenerator) -> Dict[str, Any]:
    return {"username": g.gen_username(), "password": g.gen_password()}


def gen_CheckUserResult(g: FieldGenerator) -> Dict[str, Any]:
    return {"correct": g._randbool(0.9)}


message_generators: Dict[str, Callable[[FieldGenerator], Dict[str, Any]]] = {
    "NearbyRequest": gen_NearbyRequest,
    "NearbyResult": gen_NearbyResult,
    "GetProfilesRequest": gen_GetProfilesRequest,
    "GetProfilesResult": gen_GetProfilesResult,
    "Hotel": gen_Hotel,
    "Address": gen_Address,
    "Image": gen_Image,
    "GetRecommendationsRequest": gen_GetRecommendationsRequest,
    "GetRecommendationsResult": gen_GetRecommendationsResult,
    "GetRatesRequest": gen_GetRatesRequest,
    "GetRatesResult": gen_GetRatesResult,
    "RatePlan": gen_RatePlan,
    "RoomType": gen_RoomType,
    "ReservationRequest": gen_ReservationRequest,
    "ReservationResult": gen_ReservationResult,
    "SearchRequest": gen_SearchRequest,
    "SearchResult": gen_SearchResult,
    "CheckUserRequest": gen_CheckUserRequest,
    "CheckUserResult": gen_CheckUserResult,
}
