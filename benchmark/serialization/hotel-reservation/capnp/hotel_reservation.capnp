# Hotel Reservation Cap'n Proto Schema
# Matches hotel_reservation.proto message definitions

@0xa1b2c3d4e5f6a7b8;

using Go = import "/go.capnp";
$Go.package("capnp");
$Go.import("github.com/appnet-org/arpc/benchmark/serialization/hotel-reservation/capnp");

# -----------------Geo service-----------------

struct NearbyRequest {
  lat @0 :Float32;
  lon @1 :Float32;
  latstring @2 :Text;
}

struct NearbyResult {
  hotelIds @0 :List(Text);
}

# -----------------Profile service-----------------

struct GetProfilesRequest {
  hotelIds @0 :List(Text);
  locale @1 :Text;
}

struct GetProfilesResult {
  hotels @0 :List(Hotel);
}

struct Hotel {
  id @0 :Text;
  name @1 :Text;
  phoneNumber @2 :Text;
  description @3 :Text;
  address @4 :Address;
  images @5 :List(Image);
}

struct Address {
  streetNumber @0 :Text;
  streetName @1 :Text;
  city @2 :Text;
  state @3 :Text;
  country @4 :Text;
  postalCode @5 :Text;
  lat @6 :Float32;
  lon @7 :Float32;
}

struct Image {
  url @0 :Text;
  default @1 :Bool;
}

# -----------------Recommendation service-----------------

struct GetRecommendationsRequest {
  require @0 :Text;
  lat @1 :Float64;
  lon @2 :Float64;
}

struct GetRecommendationsResult {
  hotelIds @0 :List(Text);
}

# -----------------Rate service-----------------

struct GetRatesRequest {
  hotelIds @0 :List(Text);
  inDate @1 :Text;
  outDate @2 :Text;
}

struct GetRatesResult {
  ratePlans @0 :List(RatePlan);
}

struct RatePlan {
  hotelId @0 :Text;
  code @1 :Text;
  inDate @2 :Text;
  outDate @3 :Text;
  roomType @4 :RoomType;
}

struct RoomType {
  bookableRate @0 :Float64;
  totalRate @1 :Float64;
  totalRateInclusive @2 :Float64;
  code @3 :Text;
  currency @4 :Text;
  roomDescription @5 :Text;
}

# -----------------Reservation service-----------------

struct ReservationRequest {
  customerName @0 :Text;
  hotelId @1 :List(Text);
  inDate @2 :Text;
  outDate @3 :Text;
  roomNumber @4 :Int32;
}

struct ReservationResult {
  hotelId @0 :List(Text);
}

# -----------------Search service-----------------

struct SearchRequest {
  lat @0 :Float32;
  lon @1 :Float32;
  inDate @2 :Text;
  outDate @3 :Text;
}

struct SearchResult {
  hotelIds @0 :List(Text);
}

# -----------------User service-----------------

struct CheckUserRequest {
  username @0 :Text;
  password @1 :Text;
}

struct CheckUserResult {
  correct @0 :Bool;
}