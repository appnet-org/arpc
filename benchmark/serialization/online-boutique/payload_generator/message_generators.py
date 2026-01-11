"""Message-level generators for creating JSON payloads."""

from typing import Any, Callable, Dict

from field_generator import FieldGenerator


def gen_CartItem(g: FieldGenerator) -> Dict[str, Any]:
    return {"product_id": g.gen_product_id(), "quantity": g.gen_quantity()}


def gen_AddItemRequest(g: FieldGenerator) -> Dict[str, Any]:
    return {"user_id": g.gen_user_id(), "item": gen_CartItem(g)}


def gen_EmptyCartRequest(g: FieldGenerator) -> Dict[str, Any]:
    return {"user_id": g.gen_user_id()}


def gen_GetCartRequest(g: FieldGenerator) -> Dict[str, Any]:
    return {"user_id": g.gen_user_id()}


def gen_Cart(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._skewed_len_small(dcfg.cart_items_min, dcfg.cart_items_max, small_hi=3, p_small=0.75)
    return {"user_id": g.gen_user_id(), "items": [gen_CartItem(g) for _ in range(n)]}


def gen_Empty(g: FieldGenerator) -> Dict[str, Any]:
    return {}


def gen_EmptyUser(g: FieldGenerator) -> Dict[str, Any]:
    return {"user_id": g.gen_user_id()}


def gen_ListRecommendationsRequest(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._skewed_len_small(dcfg.rec_req_ids_min, dcfg.rec_req_ids_max, small_hi=5, p_small=0.8)
    return {"user_id": g.gen_user_id(), "product_ids": [g.gen_product_id() for _ in range(n)]}


def gen_ListRecommendationsResponse(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._skewed_len_small(dcfg.rec_resp_ids_min, dcfg.rec_resp_ids_max, small_hi=5, p_small=0.8)
    return {"product_ids": [g.gen_product_id() for _ in range(n)]}


def gen_Money(g: FieldGenerator) -> Dict[str, Any]:
    # generic money, not necessarily USD
    return g.gen_money()


def gen_Product(g: FieldGenerator) -> Dict[str, Any]:
    tcfg = g.cfg.product_text
    dcfg = g.cfg.dist
    pid = g.gen_product_id()
    name, description = g.gen_product_text()
    cat_n = g._bounded_len(dcfg.product_categories_min, dcfg.product_categories_max)
    categories = [g._choice(tcfg.categories) for _ in range(cat_n)]
    return {
        "id": pid,
        "name": name,
        "description": description,
        "picture": g.gen_url("picture", token=pid),
        "price_usd": g.gen_usd_money((g.cfg.money.price_units_min, g.cfg.money.price_units_max)),
        "categories": categories,
    }


def gen_ListProductsResponse(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(dcfg.list_products_min, dcfg.list_products_max)
    return {"products": [gen_Product(g) for _ in range(n)]}


def gen_GetProductRequest(g: FieldGenerator) -> Dict[str, Any]:
    return {"id": g.gen_product_id()}


def gen_SearchProductsRequest(g: FieldGenerator) -> Dict[str, Any]:
    return {"query": g.gen_search_query()}


def gen_SearchProductsResponse(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(dcfg.search_results_min, dcfg.search_results_max)
    return {"results": [gen_Product(g) for _ in range(n)]}


def gen_Address(g: FieldGenerator) -> Dict[str, Any]:
    return g.gen_address()


def gen_GetQuoteRequest(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._skewed_len_small(dcfg.cart_items_min, dcfg.cart_items_max, small_hi=3, p_small=0.8)
    return {"address": gen_Address(g), "items": [gen_CartItem(g) for _ in range(n)]}


def gen_GetQuoteResponse(g: FieldGenerator) -> Dict[str, Any]:
    return {"cost_usd": g.gen_usd_money((g.cfg.money.shipping_units_min, g.cfg.money.shipping_units_max))}


def gen_ShipOrderRequest(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._skewed_len_small(dcfg.cart_items_min, dcfg.cart_items_max, small_hi=3, p_small=0.8)
    return {"address": gen_Address(g), "items": [gen_CartItem(g) for _ in range(n)]}


def gen_ShipOrderResponse(g: FieldGenerator) -> Dict[str, Any]:
    return {"tracking_id": g.gen_tracking_id()}


def gen_GetSupportedCurrenciesResponse(g: FieldGenerator) -> Dict[str, Any]:
    mcfg = g.cfg.money
    # 3–min(10, len(currencies))
    n = g._bounded_len(3, min(10, len(mcfg.currencies)))
    # choose unique
    codes = list(mcfg.currencies)
    g.rng.shuffle(codes)
    return {"currency_codes": codes[:n]}


def gen_CurrencyConversionRequest(g: FieldGenerator) -> Dict[str, Any]:
    mcfg = g.cfg.money
    from_code = g._choice(mcfg.currencies)
    to_code = g._choice([c for c in mcfg.currencies if c != from_code] or mcfg.currencies)
    from_amt = g.gen_money(currency_code=from_code, units_range=(mcfg.conversion_units_min, mcfg.conversion_units_max))
    return {"from": from_amt, "to_code": to_code, "user_id": g.gen_user_id()}


def gen_CreditCardInfo(g: FieldGenerator) -> Dict[str, Any]:
    return g.gen_credit_card()


def gen_ChargeRequest(g: FieldGenerator) -> Dict[str, Any]:
    amt = g.gen_money(currency_code=g._choice(g.cfg.money.currencies), units_range=(1, 2000))
    return {"amount": amt, "credit_card": gen_CreditCardInfo(g)}


def gen_ChargeResponse(g: FieldGenerator) -> Dict[str, Any]:
    return {"transaction_id": g.gen_transaction_id()}


def gen_OrderItem(g: FieldGenerator) -> Dict[str, Any]:
    # Keep minimal: cost independent of quantity (reasonable enough for payload shape).
    return {
        "item": gen_CartItem(g),
        "cost": g.gen_usd_money((1, 300)),
    }


def gen_OrderResult(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._skewed_len_small(dcfg.order_items_min, dcfg.order_items_max, small_hi=3, p_small=0.75)
    return {
        "order_id": g.gen_order_id(),
        "shipping_tracking_id": g.gen_tracking_id(),
        "shipping_cost": g.gen_usd_money((g.cfg.money.shipping_units_min, g.cfg.money.shipping_units_max)),
        "shipping_address": gen_Address(g),
        "items": [gen_OrderItem(g) for _ in range(n)],
    }


def gen_SendOrderConfirmationRequest(g: FieldGenerator) -> Dict[str, Any]:
    uid = g.gen_user_id()
    return {"email": g.gen_email(uid), "order": gen_OrderResult(g)}


def gen_PlaceOrderRequest(g: FieldGenerator) -> Dict[str, Any]:
    uid = g.gen_user_id()
    return {
        "user_id": uid,
        "user_currency": g._choice(g.cfg.money.currencies),
        "address": gen_Address(g),
        "email": g.gen_email(uid),
        "credit_card": gen_CreditCardInfo(g),
    }


def gen_PlaceOrderResponse(g: FieldGenerator) -> Dict[str, Any]:
    return {"order": gen_OrderResult(g)}


def gen_Ad(g: FieldGenerator) -> Dict[str, Any]:
    templates = (
        "Save 20% on {category} essentials",
        "Limited-time deals on {item}",
        "Upgrade your {category} setup today",
        "New arrivals: {item} collection",
    )
    tcfg = g.cfg.product_text
    text = g._choice(templates).format(category=g._choice(tcfg.categories), item=g._choice(tcfg.items).lower())
    return {"redirect_url": g.gen_url("ad"), "text": text}


def gen_AdRequest(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    tcfg = g.cfg.product_text
    n = g._bounded_len(dcfg.ad_keys_min, dcfg.ad_keys_max)
    # context keys from mixed vocab
    keys = []
    for _ in range(n):
        if g._randbool(0.5):
            keys.append(g._choice(tcfg.categories))
        else:
            keys.append(g._choice(tcfg.items).lower())
    return {"user_id": g.gen_user_id(), "context_keys": keys}


def gen_AdResponse(g: FieldGenerator) -> Dict[str, Any]:
    dcfg = g.cfg.dist
    n = g._bounded_len(dcfg.ads_min, dcfg.ads_max)
    return {"ads": [gen_Ad(g) for _ in range(n)]}


# Mapping from message name -> generator function
message_generators: Dict[str, Callable[[FieldGenerator], Dict[str, Any]]] = {
    "CartItem": gen_CartItem,
    "AddItemRequest": gen_AddItemRequest,
    "EmptyCartRequest": gen_EmptyCartRequest,
    "GetCartRequest": gen_GetCartRequest,
    "Cart": gen_Cart,
    "Empty": gen_Empty,
    "EmptyUser": gen_EmptyUser,

    "ListRecommendationsRequest": gen_ListRecommendationsRequest,
    "ListRecommendationsResponse": gen_ListRecommendationsResponse,

    "Money": gen_Money,
    "Product": gen_Product,
    "ListProductsResponse": gen_ListProductsResponse,
    "GetProductRequest": gen_GetProductRequest,
    "SearchProductsRequest": gen_SearchProductsRequest,
    "SearchProductsResponse": gen_SearchProductsResponse,

    "Address": gen_Address,
    "GetQuoteRequest": gen_GetQuoteRequest,
    "GetQuoteResponse": gen_GetQuoteResponse,
    "ShipOrderRequest": gen_ShipOrderRequest,
    "ShipOrderResponse": gen_ShipOrderResponse,

    "GetSupportedCurrenciesResponse": gen_GetSupportedCurrenciesResponse,
    "CurrencyConversionRequest": gen_CurrencyConversionRequest,

    "CreditCardInfo": gen_CreditCardInfo,
    "ChargeRequest": gen_ChargeRequest,
    "ChargeResponse": gen_ChargeResponse,

    "OrderItem": gen_OrderItem,
    "OrderResult": gen_OrderResult,
    "SendOrderConfirmationRequest": gen_SendOrderConfirmationRequest,

    "PlaceOrderRequest": gen_PlaceOrderRequest,
    "PlaceOrderResponse": gen_PlaceOrderResponse,

    "AdRequest": gen_AdRequest,
    "AdResponse": gen_AdResponse,
    "Ad": gen_Ad,
}
