"""Field-level generators for creating realistic hotel data."""

import random
import string
from datetime import datetime, timedelta
from typing import Any, Dict, Optional, Tuple

from config import Config


class FieldGenerator:
    """Generator class for creating field-level hotel data."""

    def __init__(self, cfg: Config):
        self.cfg = cfg
        self.rng = random.Random(cfg.seed)

        self.hotel_ids = [self._gen_hotel_id_raw() for _ in range(cfg.hotel_pool_size)]
        self.usernames = [self._gen_username_raw() for _ in range(cfg.user_pool_size)]

    # ---- core random helpers ----

    def _hex(self, n: int) -> str:
        return "".join(self.rng.choice("0123456789abcdef") for _ in range(n))

    def _alnum(self, n: int) -> str:
        alphabet = string.ascii_lowercase + string.digits
        return "".join(self.rng.choice(alphabet) for _ in range(n))

    def _choice(self, seq):
        return seq[self.rng.randrange(len(seq))]

    def _randint(self, a: int, b: int) -> int:
        return self.rng.randint(a, b)

    def _randfloat(self, a: float, b: float) -> float:
        return a + (b - a) * self.rng.random()

    def _randbool(self, p_true: float) -> bool:
        return self.rng.random() < p_true

    def _bounded_len(self, lo: int, hi: int) -> int:
        if lo >= hi:
            return lo
        return self._randint(lo, hi)

    def _skewed_len_small(self, lo: int, hi: int, small_hi: int, p_small: float) -> int:
        if lo >= hi:
            return lo
        if self._randbool(p_small):
            return self._randint(lo, min(small_hi, hi))
        return self._randint(max(lo, min(small_hi, hi) + 1), hi)

    # ---- ID generators ----

    def _gen_hotel_id_raw(self) -> str:
        return f"hotel_{self._hex(8)}"

    def _gen_username_raw(self) -> str:
        return f"user_{self._alnum(8)}"

    def gen_hotel_id(self) -> str:
        return self._choice(self.hotel_ids)

    def gen_username(self) -> str:
        return self._choice(self.usernames)

    # ---- geo ----

    def gen_lat(self) -> float:
        return self._randfloat(-90.0, 90.0)

    def gen_lon(self) -> float:
        return self._randfloat(-180.0, 180.0)

    def gen_latstring(self, lat: Optional[float] = None, lon: Optional[float] = None) -> str:
        if lat is None:
            lat = self.gen_lat()
        if lon is None:
            lon = self.gen_lon()
        return f"{lat:.6f},{lon:.6f}"

    # ---- dates ----

    def gen_in_date(self) -> str:
        base = datetime.utcnow().date()
        offset = self._randint(0, 365)
        d = base + timedelta(days=offset)
        return d.strftime("%Y-%m-%d")

    def gen_out_date(self, in_date: Optional[str] = None) -> str:
        if in_date is None:
            in_date = self.gen_in_date()
        base = datetime.strptime(in_date, "%Y-%m-%d").date()
        nights = self._randint(1, 14)
        out = base + timedelta(days=nights)
        return out.strftime("%Y-%m-%d")

    def gen_date_range(self) -> Tuple[str, str]:
        in_d = self.gen_in_date()
        out_d = self.gen_out_date(in_d)
        return in_d, out_d

    # ---- locale ----

    def gen_locale(self) -> str:
        return self._choice(self.cfg.hotel_text.locales)

    # ---- hotel text ----

    def gen_hotel_name(self) -> str:
        t = self.cfg.hotel_text
        return f"{self._choice(t.name_prefixes)} {self._choice(t.name_suffixes)}"

    def gen_hotel_description(self) -> str:
        return self._choice(self.cfg.hotel_text.description_templates)

    def gen_phone_number(self) -> str:
        area = self._randint(200, 999)
        exch = self._randint(200, 999)
        sub = self._randint(1000, 9999)
        return f"+1-{area}-{exch}-{sub}"

    def gen_image_url(self, hotel_id: Optional[str] = None) -> str:
        base = self.cfg.hotel_text.image_base_url
        tok = hotel_id or self._alnum(10)
        return f"{base}/{tok}.jpg"

    # ---- address (hotel Address: streetNumber, streetName, city, state, country, postalCode, lat, lon) ----

    def gen_address(self) -> Dict[str, Any]:
        acfg = self.cfg.address
        city, state = self._choice(acfg.city_state_pairs)
        street_no = str(self._randint(1, 9999))
        street_name = self._choice(acfg.street_names) + " " + self._choice(acfg.street_suffixes)
        postal_code = str(self._randint(acfg.zip_min, acfg.zip_max))
        lat = self._randfloat(24.0, 50.0)
        lon = self._randfloat(-125.0, -66.0)
        return {
            "streetNumber": street_no,
            "streetName": street_name,
            "city": city,
            "state": state,
            "country": self._choice(acfg.countries),
            "postalCode": postal_code,
            "lat": round(lat, 6),
            "lon": round(lon, 6),
        }

    # ---- room / rate ----

    def gen_room_type(self) -> Dict[str, Any]:
        rcfg = self.cfg.rate
        bookable = self._randfloat(rcfg.bookable_rate_min, rcfg.bookable_rate_max)
        total = bookable + self._randfloat(0, 50)
        total_inc = total * (1.0 + self._randfloat(0, 0.15))
        return {
            "bookableRate": round(bookable, 2),
            "totalRate": round(total, 2),
            "totalRateInclusive": round(total_inc, 2),
            "code": self._choice(rcfg.room_codes),
            "currency": self._choice(rcfg.currencies),
            "roomDescription": self._choice(rcfg.room_description_templates),
        }

    def gen_rate_plan_code(self) -> str:
        return f"RATE_{self._alnum(6)}"

    # ---- user ----

    def gen_password(self) -> str:
        return self._alnum(12)

    def gen_customer_name(self) -> str:
        t = self.cfg.hotel_text
        return f"{self._choice(t.first_names)} {self._choice(t.last_names)}"

    def gen_room_number(self) -> int:
        return self._randint(1, 500)
