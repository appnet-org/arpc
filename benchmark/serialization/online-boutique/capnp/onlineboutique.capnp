# Copyright 2020 Google LLC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

@0xb8c3d5e7f9a1b2c3;

using Go = import "/go.capnp";
$Go.package("onlineboutique");
$Go.import("github.com/appnetorg/online-boutique-arpc/proto");

# -----------------Cart service-----------------

interface CartService {
  addItem @0 (request: AddItemRequest) -> (response: Empty);
  getCart @1 (request: GetCartRequest) -> (response: Cart);
  emptyCart @2 (request: EmptyCartRequest) -> (response: Empty);
}

struct CartItem {
  productId @0 :Text;
  quantity @1 :Int32;
}

struct AddItemRequest {
  userId @0 :Text;
  item @1 :CartItem;
}

struct EmptyCartRequest {
  userId @0 :Text;
}

struct GetCartRequest {
  userId @0 :Text;
}

struct Cart {
  userId @0 :Text;
  items @1 :List(CartItem);
}

struct Empty {
}

struct EmptyUser {
  userId @0 :Text;
}

# ---------------Recommendation service----------

interface RecommendationService {
  listRecommendations @0 (request: ListRecommendationsRequest) -> (response: ListRecommendationsResponse);
}

struct ListRecommendationsRequest {
  userId @0 :Text;
  productIds @1 :List(Text);
}

struct ListRecommendationsResponse {
  productIds @0 :List(Text);
}

# ---------------Product Catalog----------------

interface ProductCatalogService {
  listProducts @0 (request: EmptyUser) -> (response: ListProductsResponse);
  getProduct @1 (request: GetProductRequest) -> (response: Product);
  searchProducts @2 (request: SearchProductsRequest) -> (response: SearchProductsResponse);
}

struct Money {
  # The 3-letter currency code defined in ISO 4217.
  currencyCode @0 :Text;

  # The whole units of the amount.
  # For example if `currencyCode` is `"USD"`, then 1 unit is one US dollar.
  units @1 :Int64;

  # Number of nano (10^-9) units of the amount.
  # The value must be between -999,999,999 and +999,999,999 inclusive.
  # If `units` is positive, `nanos` must be positive or zero.
  # If `units` is zero, `nanos` can be positive, zero, or negative.
  # If `units` is negative, `nanos` must be negative or zero.
  # For example $-1.75 is represented as `units`=-1 and `nanos`=-750,000,000.
  nanos @2 :Int32;
}

struct Product {
  id @0 :Text;
  name @1 :Text;
  description @2 :Text;
  picture @3 :Text;
  priceUsd @4 :Money;

  # Categories such as "clothing" or "kitchen" that can be used to look up
  # other related products.
  categories @5 :List(Text);
}

struct ListProductsResponse {
  products @0 :List(Product);
}

struct GetProductRequest {
  id @0 :Text;
}

struct SearchProductsRequest {
  query @0 :Text;
}

struct SearchProductsResponse {
  results @0 :List(Product);
}

# ---------------Shipping Service----------

interface ShippingService {
  getQuote @0 (request: GetQuoteRequest) -> (response: GetQuoteResponse);
  shipOrder @1 (request: ShipOrderRequest) -> (response: ShipOrderResponse);
}

struct Address {
  streetAddress @0 :Text;
  city @1 :Text;
  state @2 :Text;
  country @3 :Text;
  zipCode @4 :Int32;
}

struct GetQuoteRequest {
  address @0 :Address;
  items @1 :List(CartItem);
}

struct GetQuoteResponse {
  costUsd @0 :Money;
}

struct ShipOrderRequest {
  address @0 :Address;
  items @1 :List(CartItem);
}

struct ShipOrderResponse {
  trackingId @0 :Text;
}

# -----------------Currency service-----------------

interface CurrencyService {
  getSupportedCurrencies @0 (request: EmptyUser) -> (response: GetSupportedCurrenciesResponse);
  convert @1 (request: CurrencyConversionRequest) -> (response: Money);
}

struct GetSupportedCurrenciesResponse {
  # The 3-letter currency code defined in ISO 4217.
  currencyCodes @0 :List(Text);
}

struct CurrencyConversionRequest {
  from @0 :Money;

  # The 3-letter currency code defined in ISO 4217.
  toCode @1 :Text;

  userId @2 :Text;
}

# -------------Payment service-----------------

interface PaymentService {
  charge @0 (request: ChargeRequest) -> (response: ChargeResponse);
}

struct CreditCardInfo {
  creditCardNumber @0 :Text;
  creditCardCvv @1 :Int32;
  creditCardExpirationYear @2 :Int32;
  creditCardExpirationMonth @3 :Int32;
}

struct ChargeRequest {
  amount @0 :Money;
  creditCard @1 :CreditCardInfo;
}

struct ChargeResponse {
  transactionId @0 :Text;
}

# -------------Email service-----------------

interface EmailService {
  sendOrderConfirmation @0 (request: SendOrderConfirmationRequest) -> (response: Empty);
}

struct OrderItem {
  item @0 :CartItem;
  cost @1 :Money;
}

struct OrderResult {
  orderId @0 :Text;
  shippingTrackingId @1 :Text;
  shippingCost @2 :Money;
  shippingAddress @3 :Address;
  items @4 :List(OrderItem);
}

struct SendOrderConfirmationRequest {
  email @0 :Text;
  order @1 :OrderResult;
}

# -------------Checkout service-----------------

interface CheckoutService {
  placeOrder @0 (request: PlaceOrderRequest) -> (response: PlaceOrderResponse);
}

struct PlaceOrderRequest {
  userId @0 :Text;
  userCurrency @1 :Text;
  address @2 :Address;
  email @3 :Text;
  creditCard @4 :CreditCardInfo;
}

struct PlaceOrderResponse {
  order @0 :OrderResult;
}

# ------------Ad service------------------

interface AdService {
  getAds @0 (request: AdRequest) -> (response: AdResponse);
}

struct Ad {
  # url to redirect to when an ad is clicked.
  redirectUrl @0 :Text;

  # short advertisement text to display.
  text @1 :Text;
}

struct AdRequest {
  userId @0 :Text;

  # List of important key words from the current page describing the context.
  contextKeys @1 :List(Text);
}

struct AdResponse {
  ads @0 :List(Ad);
}
