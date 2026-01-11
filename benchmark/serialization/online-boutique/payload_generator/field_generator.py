"""Field-level generators for creating realistic data."""

import random
import string
from datetime import datetime
from typing import Any, Dict, Optional, Tuple

from config import Config


class FieldGenerator:
    """Generator class for creating field-level data."""
    
    def __init__(self, cfg: Config):
        self.cfg = cfg
        self.rng = random.Random(cfg.seed)

        # Small pools to make data "look real" without heavy cross-entity logic.
        self.user_ids = [self._gen_user_id() for _ in range(cfg.user_pool_size)]
        self.product_ids = [self._gen_product_id() for _ in range(cfg.product_pool_size)]

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

    def _randbool(self, p_true: float) -> bool:
        return self.rng.random() < p_true

    def _bounded_len(self, lo: int, hi: int) -> int:
        if lo >= hi:
            return lo
        return self._randint(lo, hi)

    def _skewed_len_small(self, lo: int, hi: int, small_hi: int, p_small: float) -> int:
        """
        Return a length in [lo, hi], skewing toward [lo, min(small_hi, hi)] with probability p_small.
        """
        if lo >= hi:
            return lo
        if self._randbool(p_small):
            return self._randint(lo, min(small_hi, hi))
        return self._randint(max(lo, min(small_hi, hi) + 1), hi)

    # ---- ID generators ----

    def _gen_user_id(self) -> str:
        return f"user_{self._hex(8)}"

    def _gen_product_id(self) -> str:
        return f"prod_{self._hex(8)}"

    def gen_user_id(self) -> str:
        return self._choice(self.user_ids)

    def gen_product_id(self) -> str:
        return self._choice(self.product_ids)

    def gen_order_id(self) -> str:
        return f"order_{self._hex(10)}"

    def gen_tracking_id(self) -> str:
        return f"tracking_{self._alnum(12)}"

    def gen_transaction_id(self) -> str:
        return f"transaction_{self._hex(10)}"

    # ---- primitive field generators ----

    def gen_quantity(self) -> int:
        qcfg = self.cfg.qty
        if self._randbool(qcfg.small_quantity_prob):
            return self._randint(qcfg.quantity_min, min(qcfg.small_quantity_max, qcfg.quantity_max))
        return self._randint(max(qcfg.small_quantity_max + 1, qcfg.quantity_min), qcfg.quantity_max)

    def gen_email(self, user_id: Optional[str] = None) -> str:
        uid = user_id or self.gen_user_id()
        return f"{uid}@example.com"

    def gen_url(self, kind: str, token: Optional[str] = None) -> str:
        tok = token or self._alnum(10)
        if kind == "picture":
            return f"{self.cfg.product_text.picture_base_url}/{tok}.jpg"
        if kind == "ad":
            return f"https://ads.example.com/click?ad={tok}"
        return f"https://example.com/{tok}"

    def gen_money(self, currency_code: Optional[str] = None, units_range: Optional[Tuple[int, int]] = None) -> Dict[str, Any]:
        mcfg = self.cfg.money
        code = currency_code or self._choice(mcfg.currencies)
        lo, hi = units_range if units_range is not None else (mcfg.price_units_min, mcfg.price_units_max)
        units_val = self._randint(lo, hi)
        nanos_val = self._choice(mcfg.nanos_choices)

        # Keep it non-negative for benchmark simplicity.
        units_json: Any = str(units_val) if mcfg.units_as_string else units_val

        return {
            "currency_code": code,
            "units": units_json,
            "nanos": nanos_val,
        }

    def gen_usd_money(self, units_range: Tuple[int, int]) -> Dict[str, Any]:
        return self.gen_money(currency_code=self.cfg.money.default_usd_code, units_range=units_range)

    def gen_address(self) -> Dict[str, Any]:
        acfg = self.cfg.address
        city, state = self._choice(acfg.city_state_pairs)

        street_no = self._randint(10, 9999)
        street_name = self._choice(("Evergreen", "Main", "Oak", "Pine", "Maple", "Cedar", "Elm", "Sunset", "Park"))
        suffix = self._choice(("St", "Ave", "Rd", "Blvd", "Ln", "Dr"))

        zip_code = self._randint(acfg.zip_min, acfg.zip_max)

        return {
            "street_address": f"{street_no} {street_name} {suffix}",
            "city": city,
            "state": state,
            "country": self._choice(acfg.countries),
            "zip_code": zip_code,
        }

    def gen_credit_card(self) -> Dict[str, Any]:
        pcfg = self.cfg.payment
        prefix = self._choice(pcfg.card_prefixes)
        # Fill to 16 digits (simple; not Luhn-valid unless you add that)
        remaining = 16 - len(prefix)
        number = prefix + "".join(self.rng.choice(string.digits) for _ in range(remaining))

        now_year = datetime.utcnow().year
        year = now_year + self._randint(pcfg.exp_years_ahead_min, pcfg.exp_years_ahead_max)
        month = self._randint(1, 12)

        return {
            "credit_card_number": number,
            "credit_card_cvv": self._randint(pcfg.cvv_min, pcfg.cvv_max),
            "credit_card_expiration_year": year,
            "credit_card_expiration_month": month,
        }

    def gen_product_text(self) -> Tuple[str, str]:
        tcfg = self.cfg.product_text
        name = f"{self._choice(tcfg.adjectives)} {self._choice(tcfg.materials)} {self._choice(tcfg.items)}"
        desc_templates = (
            "A high-quality {item} designed for everyday use.",
            "A {adj} {item} made from durable {mat} materials.",
            "This {item} combines comfort and style for modern living.",
            "Built to last, this {item} is a practical addition to your home.",
        )
        tmpl = self._choice(desc_templates)
        description = tmpl.format(
            item=self._choice(tcfg.items).lower(),
            adj=self._choice(tcfg.adjectives).lower(),
            mat=self._choice(tcfg.materials).lower(),
        )
        # Add a short second sentence sometimes.
        if self._randbool(0.5):
            description += " Easy to maintain and thoughtfully crafted."
        return name, description

    def gen_search_query(self) -> str:
        tcfg = self.cfg.product_text
        # 1–3 keywords from mixed vocab
        terms = []
        terms.append(self._choice(tcfg.items).lower())
        if self._randbool(0.7):
            terms.append(self._choice(tcfg.materials).lower())
        if self._randbool(0.4):
            terms.append(self._choice(tcfg.categories))
        return " ".join(terms[: self._randint(1, min(3, len(terms)))])
