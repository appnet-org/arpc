# Payload Generator

Generates JSON payloads for Hotel Reservation message types (hotel_reservation.proto). Useful for testing and benchmarking serialization formats.

## Running

From this directory:

```bash
python main.py
```

Or:

```bash
python -m payload_generator.main
```

## Configuration

Edit `config.py` to customize:

- **`seed`**: Random seed for deterministic generation (default: `1`)
- **`counts`**: Number of payloads per message type (default: ~60k total across 19 types)
- **`output.out_dir`**: Output directory (default: `../payloads/`)
- **`output.pretty`**: Pretty-print JSON instead of JSONL (default: `False`)

Other settings control field generation (addresses, hotel names, rates, dates, etc.).

## Output

Generates one file per message type in the output directory:

- JSONL format (one JSON object per line) by default
- Pretty JSON array if `output.pretty = True`

Message types: NearbyRequest, NearbyResult, GetProfilesRequest, GetProfilesResult, Hotel, Address, Image, GetRecommendationsRequest, GetRecommendationsResult, GetRatesRequest, GetRatesResult, RatePlan, RoomType, ReservationRequest, ReservationResult, SearchRequest, SearchResult, CheckUserRequest, CheckUserResult.
