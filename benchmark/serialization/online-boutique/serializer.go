package main

import (
	"fmt"

	"capnproto.org/go/capnp/v3"
	onlineboutique_capnp "github.com/appnet-org/arpc/benchmark/serialization/online-boutique/capnp"
	onlineboutique_flat "github.com/appnet-org/arpc/benchmark/serialization/online-boutique/flatbuffers/onlineboutique"
	onlineboutique "github.com/appnet-org/arpc/benchmark/serialization/online-boutique/proto"
	flatbuffers "github.com/google/flatbuffers/go"
	"google.golang.org/protobuf/proto"
)

// serializeProto serializes a proto message to protobuf format
func serializeProto(msg proto.Message) ([]byte, error) {
	return proto.Marshal(msg)
}

// serializeSymphony serializes a proto message to Symphony format
func serializeSymphony(msg proto.Message) ([]byte, error) {
	// Use type switch to call the appropriate MarshalSymphony method
	switch m := msg.(type) {
	case *onlineboutique.CartItem:
		return m.MarshalSymphony()
	case *onlineboutique.AddItemRequest:
		return m.MarshalSymphony()
	case *onlineboutique.EmptyCartRequest:
		return m.MarshalSymphony()
	case *onlineboutique.GetCartRequest:
		return m.MarshalSymphony()
	case *onlineboutique.Cart:
		return m.MarshalSymphony()
	case *onlineboutique.Empty:
		return m.MarshalSymphony()
	case *onlineboutique.EmptyUser:
		return m.MarshalSymphony()
	case *onlineboutique.ListRecommendationsRequest:
		return m.MarshalSymphony()
	case *onlineboutique.ListRecommendationsResponse:
		return m.MarshalSymphony()
	case *onlineboutique.Product:
		return m.MarshalSymphony()
	case *onlineboutique.ListProductsResponse:
		return m.MarshalSymphony()
	case *onlineboutique.GetProductRequest:
		return m.MarshalSymphony()
	case *onlineboutique.SearchProductsRequest:
		return m.MarshalSymphony()
	case *onlineboutique.SearchProductsResponse:
		return m.MarshalSymphony()
	case *onlineboutique.GetQuoteRequest:
		return m.MarshalSymphony()
	case *onlineboutique.GetQuoteResponse:
		return m.MarshalSymphony()
	case *onlineboutique.ShipOrderRequest:
		return m.MarshalSymphony()
	case *onlineboutique.ShipOrderResponse:
		return m.MarshalSymphony()
	case *onlineboutique.Address:
		return m.MarshalSymphony()
	case *onlineboutique.Money:
		return m.MarshalSymphony()
	case *onlineboutique.GetSupportedCurrenciesResponse:
		return m.MarshalSymphony()
	case *onlineboutique.CurrencyConversionRequest:
		return m.MarshalSymphony()
	case *onlineboutique.CreditCardInfo:
		return m.MarshalSymphony()
	case *onlineboutique.ChargeRequest:
		return m.MarshalSymphony()
	case *onlineboutique.ChargeResponse:
		return m.MarshalSymphony()
	case *onlineboutique.OrderItem:
		return m.MarshalSymphony()
	case *onlineboutique.OrderResult:
		return m.MarshalSymphony()
	case *onlineboutique.SendOrderConfirmationRequest:
		return m.MarshalSymphony()
	case *onlineboutique.PlaceOrderRequest:
		return m.MarshalSymphony()
	case *onlineboutique.PlaceOrderResponse:
		return m.MarshalSymphony()
	case *onlineboutique.AdRequest:
		return m.MarshalSymphony()
	case *onlineboutique.AdResponse:
		return m.MarshalSymphony()
	case *onlineboutique.Ad:
		return m.MarshalSymphony()
	default:
		panic(fmt.Sprintf("unsupported message type for Symphony: %T", msg))
	}
}

// serializeSymphonyHybrid serializes a proto message to Symphony Hybrid format
func serializeSymphonyHybrid(msg proto.Message) ([]byte, error) {
	// Use type switch to call the appropriate MarshalSymphonyHybrid method
	switch m := msg.(type) {
	case *onlineboutique.CartItem:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.AddItemRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.EmptyCartRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.GetCartRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.Cart:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.Empty:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.EmptyUser:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.ListRecommendationsRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.ListRecommendationsResponse:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.Product:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.ListProductsResponse:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.GetProductRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.SearchProductsRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.SearchProductsResponse:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.GetQuoteRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.GetQuoteResponse:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.ShipOrderRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.ShipOrderResponse:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.Address:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.Money:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.GetSupportedCurrenciesResponse:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.CurrencyConversionRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.CreditCardInfo:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.ChargeRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.ChargeResponse:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.OrderItem:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.OrderResult:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.SendOrderConfirmationRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.PlaceOrderRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.PlaceOrderResponse:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.AdRequest:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.AdResponse:
		return m.MarshalSymphonyHybrid()
	case *onlineboutique.Ad:
		return m.MarshalSymphonyHybrid()
	default:
		panic(fmt.Sprintf("unsupported message type for Symphony Hybrid: %T", msg))
	}
}

// serializeFlatbuffers serializes a proto message to FlatBuffers format
func serializeFlatbuffers(msg proto.Message) ([]byte, error) {
	builder := flatbuffers.NewBuilder(0)
	var offset flatbuffers.UOffsetT
	var err error

	switch m := msg.(type) {
	case *onlineboutique.Product:
		offset, err = serializeProductFlatbuffers(builder, m)
	case *onlineboutique.CartItem:
		offset, err = serializeCartItemFlatbuffers(builder, m)
	case *onlineboutique.Money:
		offset, err = serializeMoneyFlatbuffers(builder, m)
	case *onlineboutique.Address:
		offset, err = serializeAddressFlatbuffers(builder, m)
	case *onlineboutique.GetProductRequest:
		id := builder.CreateString(m.Id)
		onlineboutique_flat.GetProductRequestStart(builder)
		onlineboutique_flat.GetProductRequestAddId(builder, id)
		offset = onlineboutique_flat.GetProductRequestEnd(builder)
	case *onlineboutique.SearchProductsRequest:
		query := builder.CreateString(m.Query)
		onlineboutique_flat.SearchProductsRequestStart(builder)
		onlineboutique_flat.SearchProductsRequestAddQuery(builder, query)
		offset = onlineboutique_flat.SearchProductsRequestEnd(builder)
	case *onlineboutique.Empty:
		onlineboutique_flat.EmptyStart(builder)
		offset = onlineboutique_flat.EmptyEnd(builder)
	case *onlineboutique.EmptyUser:
		userId := builder.CreateString(m.UserId)
		onlineboutique_flat.EmptyUserStart(builder)
		onlineboutique_flat.EmptyUserAddUserId(builder, userId)
		offset = onlineboutique_flat.EmptyUserEnd(builder)
	case *onlineboutique.GetCartRequest:
		userId := builder.CreateString(m.UserId)
		onlineboutique_flat.GetCartRequestStart(builder)
		onlineboutique_flat.GetCartRequestAddUserId(builder, userId)
		offset = onlineboutique_flat.GetCartRequestEnd(builder)
	case *onlineboutique.EmptyCartRequest:
		userId := builder.CreateString(m.UserId)
		onlineboutique_flat.EmptyCartRequestStart(builder)
		onlineboutique_flat.EmptyCartRequestAddUserId(builder, userId)
		offset = onlineboutique_flat.EmptyCartRequestEnd(builder)
	case *onlineboutique.Ad:
		offset, err = serializeAdFlatbuffers(builder, m)
	case *onlineboutique.AddItemRequest:
		offset, err = serializeAddItemRequestFlatbuffers(builder, m)
	case *onlineboutique.AdRequest:
		offset, err = serializeAdRequestFlatbuffers(builder, m)
	case *onlineboutique.AdResponse:
		offset, err = serializeAdResponseFlatbuffers(builder, m)
	case *onlineboutique.Cart:
		offset, err = serializeCartFlatbuffers(builder, m)
	case *onlineboutique.ChargeRequest:
		offset, err = serializeChargeRequestFlatbuffers(builder, m)
	case *onlineboutique.ChargeResponse:
		transactionId := builder.CreateString(m.TransactionId)
		onlineboutique_flat.ChargeResponseStart(builder)
		onlineboutique_flat.ChargeResponseAddTransactionId(builder, transactionId)
		offset = onlineboutique_flat.ChargeResponseEnd(builder)
	case *onlineboutique.CreditCardInfo:
		offset, err = serializeCreditCardInfoFlatbuffers(builder, m)
	case *onlineboutique.CurrencyConversionRequest:
		var fromOffset flatbuffers.UOffsetT
		if m.From != nil {
			fromOffset, _ = serializeMoneyFlatbuffers(builder, m.From)
		}
		toCode := builder.CreateString(m.ToCode)
		userId := builder.CreateString(m.UserId)
		onlineboutique_flat.CurrencyConversionRequestStart(builder)
		if m.From != nil {
			onlineboutique_flat.CurrencyConversionRequestAddFrom(builder, fromOffset)
		}
		onlineboutique_flat.CurrencyConversionRequestAddToCode(builder, toCode)
		onlineboutique_flat.CurrencyConversionRequestAddUserId(builder, userId)
		offset = onlineboutique_flat.CurrencyConversionRequestEnd(builder)
	case *onlineboutique.GetQuoteRequest:
		var addressOffset flatbuffers.UOffsetT
		if m.Address != nil {
			addressOffset, _ = serializeAddressFlatbuffers(builder, m.Address)
		}
		var itemsOffset flatbuffers.UOffsetT
		if len(m.Items) > 0 {
			itemOffsets := make([]flatbuffers.UOffsetT, len(m.Items))
			for i, item := range m.Items {
				itemOffsets[i], _ = serializeCartItemFlatbuffers(builder, item)
			}
			itemsOffset = onlineboutique_flat.GetQuoteRequestStartItemsVector(builder, len(m.Items))
			for i := len(m.Items) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(itemOffsets[i])
			}
			itemsOffset = builder.EndVector(len(m.Items))
		}
		onlineboutique_flat.GetQuoteRequestStart(builder)
		if m.Address != nil {
			onlineboutique_flat.GetQuoteRequestAddAddress(builder, addressOffset)
		}
		if len(m.Items) > 0 {
			onlineboutique_flat.GetQuoteRequestAddItems(builder, itemsOffset)
		}
		offset = onlineboutique_flat.GetQuoteRequestEnd(builder)
	case *onlineboutique.GetQuoteResponse:
		var costUsdOffset flatbuffers.UOffsetT
		if m.CostUsd != nil {
			costUsdOffset, _ = serializeMoneyFlatbuffers(builder, m.CostUsd)
		}
		onlineboutique_flat.GetQuoteResponseStart(builder)
		if m.CostUsd != nil {
			onlineboutique_flat.GetQuoteResponseAddCostUsd(builder, costUsdOffset)
		}
		offset = onlineboutique_flat.GetQuoteResponseEnd(builder)
	case *onlineboutique.GetSupportedCurrenciesResponse:
		var currencyCodesOffset flatbuffers.UOffsetT
		if len(m.CurrencyCodes) > 0 {
			codeOffsets := make([]flatbuffers.UOffsetT, len(m.CurrencyCodes))
			for i, code := range m.CurrencyCodes {
				codeOffsets[i] = builder.CreateString(code)
			}
			currencyCodesOffset = onlineboutique_flat.GetSupportedCurrenciesResponseStartCurrencyCodesVector(builder, len(m.CurrencyCodes))
			for i := len(m.CurrencyCodes) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(codeOffsets[i])
			}
			currencyCodesOffset = builder.EndVector(len(m.CurrencyCodes))
		}
		onlineboutique_flat.GetSupportedCurrenciesResponseStart(builder)
		if len(m.CurrencyCodes) > 0 {
			onlineboutique_flat.GetSupportedCurrenciesResponseAddCurrencyCodes(builder, currencyCodesOffset)
		}
		offset = onlineboutique_flat.GetSupportedCurrenciesResponseEnd(builder)
	case *onlineboutique.ListProductsResponse:
		var productsOffset flatbuffers.UOffsetT
		if len(m.Products) > 0 {
			productOffsets := make([]flatbuffers.UOffsetT, len(m.Products))
			for i, product := range m.Products {
				productOffsets[i], _ = serializeProductFlatbuffers(builder, product)
			}
			productsOffset = onlineboutique_flat.ListProductsResponseStartProductsVector(builder, len(m.Products))
			for i := len(m.Products) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(productOffsets[i])
			}
			productsOffset = builder.EndVector(len(m.Products))
		}
		onlineboutique_flat.ListProductsResponseStart(builder)
		if len(m.Products) > 0 {
			onlineboutique_flat.ListProductsResponseAddProducts(builder, productsOffset)
		}
		offset = onlineboutique_flat.ListProductsResponseEnd(builder)
	case *onlineboutique.ListRecommendationsRequest:
		userId := builder.CreateString(m.UserId)
		var productIdsOffset flatbuffers.UOffsetT
		if len(m.ProductIds) > 0 {
			productIdOffsets := make([]flatbuffers.UOffsetT, len(m.ProductIds))
			for i, id := range m.ProductIds {
				productIdOffsets[i] = builder.CreateString(id)
			}
			productIdsOffset = onlineboutique_flat.ListRecommendationsRequestStartProductIdsVector(builder, len(m.ProductIds))
			for i := len(m.ProductIds) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(productIdOffsets[i])
			}
			productIdsOffset = builder.EndVector(len(m.ProductIds))
		}
		onlineboutique_flat.ListRecommendationsRequestStart(builder)
		onlineboutique_flat.ListRecommendationsRequestAddUserId(builder, userId)
		if len(m.ProductIds) > 0 {
			onlineboutique_flat.ListRecommendationsRequestAddProductIds(builder, productIdsOffset)
		}
		offset = onlineboutique_flat.ListRecommendationsRequestEnd(builder)
	case *onlineboutique.ListRecommendationsResponse:
		var productIdsOffset flatbuffers.UOffsetT
		if len(m.ProductIds) > 0 {
			productIdOffsets := make([]flatbuffers.UOffsetT, len(m.ProductIds))
			for i, id := range m.ProductIds {
				productIdOffsets[i] = builder.CreateString(id)
			}
			productIdsOffset = onlineboutique_flat.ListRecommendationsResponseStartProductIdsVector(builder, len(m.ProductIds))
			for i := len(m.ProductIds) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(productIdOffsets[i])
			}
			productIdsOffset = builder.EndVector(len(m.ProductIds))
		}
		onlineboutique_flat.ListRecommendationsResponseStart(builder)
		if len(m.ProductIds) > 0 {
			onlineboutique_flat.ListRecommendationsResponseAddProductIds(builder, productIdsOffset)
		}
		offset = onlineboutique_flat.ListRecommendationsResponseEnd(builder)
	case *onlineboutique.OrderItem:
		offset, err = serializeOrderItemFlatbuffers(builder, m)
	case *onlineboutique.OrderResult:
		offset, err = serializeOrderResultFlatbuffers(builder, m)
	case *onlineboutique.PlaceOrderRequest:
		offset, err = serializePlaceOrderRequestFlatbuffers(builder, m)
	case *onlineboutique.PlaceOrderResponse:
		offset, err = serializePlaceOrderResponseFlatbuffers(builder, m)
	case *onlineboutique.SearchProductsResponse:
		var resultsOffset flatbuffers.UOffsetT
		if len(m.Results) > 0 {
			resultOffsets := make([]flatbuffers.UOffsetT, len(m.Results))
			for i, product := range m.Results {
				resultOffsets[i], _ = serializeProductFlatbuffers(builder, product)
			}
			resultsOffset = onlineboutique_flat.SearchProductsResponseStartResultsVector(builder, len(m.Results))
			for i := len(m.Results) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(resultOffsets[i])
			}
			resultsOffset = builder.EndVector(len(m.Results))
		}
		onlineboutique_flat.SearchProductsResponseStart(builder)
		if len(m.Results) > 0 {
			onlineboutique_flat.SearchProductsResponseAddResults(builder, resultsOffset)
		}
		offset = onlineboutique_flat.SearchProductsResponseEnd(builder)
	case *onlineboutique.SendOrderConfirmationRequest:
		offset, err = serializeSendOrderConfirmationRequestFlatbuffers(builder, m)
	case *onlineboutique.ShipOrderRequest:
		offset, err = serializeShipOrderRequestFlatbuffers(builder, m)
	case *onlineboutique.ShipOrderResponse:
		trackingId := builder.CreateString(m.TrackingId)
		onlineboutique_flat.ShipOrderResponseStart(builder)
		onlineboutique_flat.ShipOrderResponseAddTrackingId(builder, trackingId)
		offset = onlineboutique_flat.ShipOrderResponseEnd(builder)
	default:
		panic(fmt.Sprintf("unsupported message type for FlatBuffers: %T", msg))
	}

	if err != nil {
		panic(fmt.Sprintf("FlatBuffers serialization error: %v", err))
	}

	builder.Finish(offset)
	return builder.FinishedBytes(), nil
}

// serializeProductFlatbuffers serializes a Product message to FlatBuffers
func serializeProductFlatbuffers(builder *flatbuffers.Builder, p *onlineboutique.Product) (flatbuffers.UOffsetT, error) {
	// Create all strings first (before starting any objects)
	id := builder.CreateString(p.Id)
	name := builder.CreateString(p.Name)
	description := builder.CreateString(p.Description)
	picture := builder.CreateString(p.Picture)

	// Create category strings
	var categoryOffsets []flatbuffers.UOffsetT
	if len(p.Categories) > 0 {
		categoryOffsets = make([]flatbuffers.UOffsetT, len(p.Categories))
		for i, cat := range p.Categories {
			categoryOffsets[i] = builder.CreateString(cat)
		}
	}

	// Serialize nested Money (creates its own string internally, then builds object)
	// Note: serializeMoneyFlatbuffers creates the currencyCode string before starting Money object
	var priceUsdOffset flatbuffers.UOffsetT
	if p.PriceUsd != nil {
		priceUsdOffset, _ = serializeMoneyFlatbuffers(builder, p.PriceUsd)
	}

	// Create categories vector (after all strings are created and nested objects are finished)
	// The category strings were already created above, now we just reference them
	var categoriesOffset flatbuffers.UOffsetT
	if len(p.Categories) > 0 {
		categoriesOffset = onlineboutique_flat.ProductStartCategoriesVector(builder, len(p.Categories))
		// Prepend in reverse order (FlatBuffers requirement)
		for i := len(p.Categories) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(categoryOffsets[i])
		}
		categoriesOffset = builder.EndVector(len(p.Categories))
	}

	// Now start the Product object
	onlineboutique_flat.ProductStart(builder)
	onlineboutique_flat.ProductAddId(builder, id)
	onlineboutique_flat.ProductAddName(builder, name)
	onlineboutique_flat.ProductAddDescription(builder, description)
	onlineboutique_flat.ProductAddPicture(builder, picture)
	if p.PriceUsd != nil {
		onlineboutique_flat.ProductAddPriceUsd(builder, priceUsdOffset)
	}
	if len(p.Categories) > 0 {
		onlineboutique_flat.ProductAddCategories(builder, categoriesOffset)
	}
	return onlineboutique_flat.ProductEnd(builder), nil
}

// serializeCartItemFlatbuffers serializes a CartItem message to FlatBuffers
func serializeCartItemFlatbuffers(builder *flatbuffers.Builder, c *onlineboutique.CartItem) (flatbuffers.UOffsetT, error) {
	productId := builder.CreateString(c.ProductId)
	onlineboutique_flat.CartItemStart(builder)
	onlineboutique_flat.CartItemAddProductId(builder, productId)
	onlineboutique_flat.CartItemAddQuantity(builder, c.Quantity)
	return onlineboutique_flat.CartItemEnd(builder), nil
}

// serializeMoneyFlatbuffers serializes a Money message to FlatBuffers
func serializeMoneyFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.Money) (flatbuffers.UOffsetT, error) {
	currencyCode := builder.CreateString(m.CurrencyCode)
	onlineboutique_flat.MoneyStart(builder)
	onlineboutique_flat.MoneyAddCurrencyCode(builder, currencyCode)
	onlineboutique_flat.MoneyAddUnits(builder, m.Units)
	onlineboutique_flat.MoneyAddNanos(builder, m.Nanos)
	return onlineboutique_flat.MoneyEnd(builder), nil
}

// serializeAddressFlatbuffers serializes an Address message to FlatBuffers
func serializeAddressFlatbuffers(builder *flatbuffers.Builder, a *onlineboutique.Address) (flatbuffers.UOffsetT, error) {
	streetAddress := builder.CreateString(a.StreetAddress)
	city := builder.CreateString(a.City)
	state := builder.CreateString(a.State)
	country := builder.CreateString(a.Country)
	onlineboutique_flat.AddressStart(builder)
	onlineboutique_flat.AddressAddStreetAddress(builder, streetAddress)
	onlineboutique_flat.AddressAddCity(builder, city)
	onlineboutique_flat.AddressAddState(builder, state)
	onlineboutique_flat.AddressAddCountry(builder, country)
	onlineboutique_flat.AddressAddZipCode(builder, a.ZipCode)
	return onlineboutique_flat.AddressEnd(builder), nil
}

// serializeAdFlatbuffers serializes an Ad message to FlatBuffers
func serializeAdFlatbuffers(builder *flatbuffers.Builder, a *onlineboutique.Ad) (flatbuffers.UOffsetT, error) {
	redirectUrl := builder.CreateString(a.RedirectUrl)
	text := builder.CreateString(a.Text)
	onlineboutique_flat.AdStart(builder)
	onlineboutique_flat.AdAddRedirectUrl(builder, redirectUrl)
	onlineboutique_flat.AdAddText(builder, text)
	return onlineboutique_flat.AdEnd(builder), nil
}

// serializeAddItemRequestFlatbuffers serializes an AddItemRequest message to FlatBuffers
func serializeAddItemRequestFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.AddItemRequest) (flatbuffers.UOffsetT, error) {
	userId := builder.CreateString(m.UserId)
	var itemOffset flatbuffers.UOffsetT
	if m.Item != nil {
		itemOffset, _ = serializeCartItemFlatbuffers(builder, m.Item)
	}
	onlineboutique_flat.AddItemRequestStart(builder)
	onlineboutique_flat.AddItemRequestAddUserId(builder, userId)
	if m.Item != nil {
		onlineboutique_flat.AddItemRequestAddItem(builder, itemOffset)
	}
	return onlineboutique_flat.AddItemRequestEnd(builder), nil
}

// serializeAdRequestFlatbuffers serializes an AdRequest message to FlatBuffers
func serializeAdRequestFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.AdRequest) (flatbuffers.UOffsetT, error) {
	userId := builder.CreateString(m.UserId)
	var contextKeysOffset flatbuffers.UOffsetT
	if len(m.ContextKeys) > 0 {
		contextKeyOffsets := make([]flatbuffers.UOffsetT, len(m.ContextKeys))
		for i, key := range m.ContextKeys {
			contextKeyOffsets[i] = builder.CreateString(key)
		}
		contextKeysOffset = onlineboutique_flat.AdRequestStartContextKeysVector(builder, len(m.ContextKeys))
		for i := len(m.ContextKeys) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(contextKeyOffsets[i])
		}
		contextKeysOffset = builder.EndVector(len(m.ContextKeys))
	}
	onlineboutique_flat.AdRequestStart(builder)
	onlineboutique_flat.AdRequestAddUserId(builder, userId)
	if len(m.ContextKeys) > 0 {
		onlineboutique_flat.AdRequestAddContextKeys(builder, contextKeysOffset)
	}
	return onlineboutique_flat.AdRequestEnd(builder), nil
}

// serializeAdResponseFlatbuffers serializes an AdResponse message to FlatBuffers
func serializeAdResponseFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.AdResponse) (flatbuffers.UOffsetT, error) {
	var adsOffset flatbuffers.UOffsetT
	if len(m.Ads) > 0 {
		adOffsets := make([]flatbuffers.UOffsetT, len(m.Ads))
		for i := range m.Ads {
			adOffsets[i], _ = serializeAdFlatbuffers(builder, m.Ads[i])
		}
		adsOffset = onlineboutique_flat.AdResponseStartAdsVector(builder, len(m.Ads))
		for i := len(m.Ads) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(adOffsets[i])
		}
		adsOffset = builder.EndVector(len(m.Ads))
	}
	onlineboutique_flat.AdResponseStart(builder)
	if len(m.Ads) > 0 {
		onlineboutique_flat.AdResponseAddAds(builder, adsOffset)
	}
	return onlineboutique_flat.AdResponseEnd(builder), nil
}

// serializeCartFlatbuffers serializes a Cart message to FlatBuffers
func serializeCartFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.Cart) (flatbuffers.UOffsetT, error) {
	userId := builder.CreateString(m.UserId)
	var itemsOffset flatbuffers.UOffsetT
	if len(m.Items) > 0 {
		itemOffsets := make([]flatbuffers.UOffsetT, len(m.Items))
		for i, item := range m.Items {
			itemOffsets[i], _ = serializeCartItemFlatbuffers(builder, item)
		}
		itemsOffset = onlineboutique_flat.CartStartItemsVector(builder, len(m.Items))
		for i := len(m.Items) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(itemOffsets[i])
		}
		itemsOffset = builder.EndVector(len(m.Items))
	}
	onlineboutique_flat.CartStart(builder)
	onlineboutique_flat.CartAddUserId(builder, userId)
	if len(m.Items) > 0 {
		onlineboutique_flat.CartAddItems(builder, itemsOffset)
	}
	return onlineboutique_flat.CartEnd(builder), nil
}

// serializeCreditCardInfoFlatbuffers serializes a CreditCardInfo message to FlatBuffers
func serializeCreditCardInfoFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.CreditCardInfo) (flatbuffers.UOffsetT, error) {
	cardNumber := builder.CreateString(m.CreditCardNumber)
	onlineboutique_flat.CreditCardInfoStart(builder)
	onlineboutique_flat.CreditCardInfoAddCreditCardNumber(builder, cardNumber)
	onlineboutique_flat.CreditCardInfoAddCreditCardCvv(builder, m.CreditCardCvv)
	onlineboutique_flat.CreditCardInfoAddCreditCardExpirationYear(builder, m.CreditCardExpirationYear)
	onlineboutique_flat.CreditCardInfoAddCreditCardExpirationMonth(builder, m.CreditCardExpirationMonth)
	return onlineboutique_flat.CreditCardInfoEnd(builder), nil
}

// serializeChargeRequestFlatbuffers serializes a ChargeRequest message to FlatBuffers
func serializeChargeRequestFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.ChargeRequest) (flatbuffers.UOffsetT, error) {
	var amountOffset flatbuffers.UOffsetT
	if m.Amount != nil {
		amountOffset, _ = serializeMoneyFlatbuffers(builder, m.Amount)
	}
	var creditCardOffset flatbuffers.UOffsetT
	if m.CreditCard != nil {
		creditCardOffset, _ = serializeCreditCardInfoFlatbuffers(builder, m.CreditCard)
	}
	onlineboutique_flat.ChargeRequestStart(builder)
	if m.Amount != nil {
		onlineboutique_flat.ChargeRequestAddAmount(builder, amountOffset)
	}
	if m.CreditCard != nil {
		onlineboutique_flat.ChargeRequestAddCreditCard(builder, creditCardOffset)
	}
	return onlineboutique_flat.ChargeRequestEnd(builder), nil
}

// serializeOrderItemFlatbuffers serializes an OrderItem message to FlatBuffers
func serializeOrderItemFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.OrderItem) (flatbuffers.UOffsetT, error) {
	var itemOffset flatbuffers.UOffsetT
	if m.Item != nil {
		itemOffset, _ = serializeCartItemFlatbuffers(builder, m.Item)
	}
	var costOffset flatbuffers.UOffsetT
	if m.Cost != nil {
		costOffset, _ = serializeMoneyFlatbuffers(builder, m.Cost)
	}
	onlineboutique_flat.OrderItemStart(builder)
	if m.Item != nil {
		onlineboutique_flat.OrderItemAddItem(builder, itemOffset)
	}
	if m.Cost != nil {
		onlineboutique_flat.OrderItemAddCost(builder, costOffset)
	}
	return onlineboutique_flat.OrderItemEnd(builder), nil
}

// serializeOrderResultFlatbuffers serializes an OrderResult message to FlatBuffers
func serializeOrderResultFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.OrderResult) (flatbuffers.UOffsetT, error) {
	orderId := builder.CreateString(m.OrderId)
	shippingTrackingId := builder.CreateString(m.ShippingTrackingId)
	var shippingCostOffset flatbuffers.UOffsetT
	if m.ShippingCost != nil {
		shippingCostOffset, _ = serializeMoneyFlatbuffers(builder, m.ShippingCost)
	}
	var shippingAddressOffset flatbuffers.UOffsetT
	if m.ShippingAddress != nil {
		shippingAddressOffset, _ = serializeAddressFlatbuffers(builder, m.ShippingAddress)
	}
	var itemsOffset flatbuffers.UOffsetT
	if len(m.Items) > 0 {
		itemOffsets := make([]flatbuffers.UOffsetT, len(m.Items))
		for i, item := range m.Items {
			itemOffsets[i], _ = serializeOrderItemFlatbuffers(builder, item)
		}
		itemsOffset = onlineboutique_flat.OrderResultStartItemsVector(builder, len(m.Items))
		for i := len(m.Items) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(itemOffsets[i])
		}
		itemsOffset = builder.EndVector(len(m.Items))
	}
	onlineboutique_flat.OrderResultStart(builder)
	onlineboutique_flat.OrderResultAddOrderId(builder, orderId)
	onlineboutique_flat.OrderResultAddShippingTrackingId(builder, shippingTrackingId)
	if m.ShippingCost != nil {
		onlineboutique_flat.OrderResultAddShippingCost(builder, shippingCostOffset)
	}
	if m.ShippingAddress != nil {
		onlineboutique_flat.OrderResultAddShippingAddress(builder, shippingAddressOffset)
	}
	if len(m.Items) > 0 {
		onlineboutique_flat.OrderResultAddItems(builder, itemsOffset)
	}
	return onlineboutique_flat.OrderResultEnd(builder), nil
}

// serializePlaceOrderRequestFlatbuffers serializes a PlaceOrderRequest message to FlatBuffers
func serializePlaceOrderRequestFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.PlaceOrderRequest) (flatbuffers.UOffsetT, error) {
	userId := builder.CreateString(m.UserId)
	userCurrency := builder.CreateString(m.UserCurrency)
	var addressOffset flatbuffers.UOffsetT
	if m.Address != nil {
		addressOffset, _ = serializeAddressFlatbuffers(builder, m.Address)
	}
	email := builder.CreateString(m.Email)
	var creditCardOffset flatbuffers.UOffsetT
	if m.CreditCard != nil {
		creditCardOffset, _ = serializeCreditCardInfoFlatbuffers(builder, m.CreditCard)
	}
	onlineboutique_flat.PlaceOrderRequestStart(builder)
	onlineboutique_flat.PlaceOrderRequestAddUserId(builder, userId)
	onlineboutique_flat.PlaceOrderRequestAddUserCurrency(builder, userCurrency)
	if m.Address != nil {
		onlineboutique_flat.PlaceOrderRequestAddAddress(builder, addressOffset)
	}
	onlineboutique_flat.PlaceOrderRequestAddEmail(builder, email)
	if m.CreditCard != nil {
		onlineboutique_flat.PlaceOrderRequestAddCreditCard(builder, creditCardOffset)
	}
	return onlineboutique_flat.PlaceOrderRequestEnd(builder), nil
}

// serializePlaceOrderResponseFlatbuffers serializes a PlaceOrderResponse message to FlatBuffers
func serializePlaceOrderResponseFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.PlaceOrderResponse) (flatbuffers.UOffsetT, error) {
	var orderOffset flatbuffers.UOffsetT
	if m.Order != nil {
		orderOffset, _ = serializeOrderResultFlatbuffers(builder, m.Order)
	}
	onlineboutique_flat.PlaceOrderResponseStart(builder)
	if m.Order != nil {
		onlineboutique_flat.PlaceOrderResponseAddOrder(builder, orderOffset)
	}
	return onlineboutique_flat.PlaceOrderResponseEnd(builder), nil
}

// serializeSendOrderConfirmationRequestFlatbuffers serializes a SendOrderConfirmationRequest message to FlatBuffers
func serializeSendOrderConfirmationRequestFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.SendOrderConfirmationRequest) (flatbuffers.UOffsetT, error) {
	email := builder.CreateString(m.Email)
	var orderOffset flatbuffers.UOffsetT
	if m.Order != nil {
		orderOffset, _ = serializeOrderResultFlatbuffers(builder, m.Order)
	}
	onlineboutique_flat.SendOrderConfirmationRequestStart(builder)
	onlineboutique_flat.SendOrderConfirmationRequestAddEmail(builder, email)
	if m.Order != nil {
		onlineboutique_flat.SendOrderConfirmationRequestAddOrder(builder, orderOffset)
	}
	return onlineboutique_flat.SendOrderConfirmationRequestEnd(builder), nil
}

// serializeShipOrderRequestFlatbuffers serializes a ShipOrderRequest message to FlatBuffers
func serializeShipOrderRequestFlatbuffers(builder *flatbuffers.Builder, m *onlineboutique.ShipOrderRequest) (flatbuffers.UOffsetT, error) {
	var addressOffset flatbuffers.UOffsetT
	if m.Address != nil {
		addressOffset, _ = serializeAddressFlatbuffers(builder, m.Address)
	}
	var itemsOffset flatbuffers.UOffsetT
	if len(m.Items) > 0 {
		itemOffsets := make([]flatbuffers.UOffsetT, len(m.Items))
		for i, item := range m.Items {
			itemOffsets[i], _ = serializeCartItemFlatbuffers(builder, item)
		}
		itemsOffset = onlineboutique_flat.ShipOrderRequestStartItemsVector(builder, len(m.Items))
		for i := len(m.Items) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(itemOffsets[i])
		}
		itemsOffset = builder.EndVector(len(m.Items))
	}
	onlineboutique_flat.ShipOrderRequestStart(builder)
	if m.Address != nil {
		onlineboutique_flat.ShipOrderRequestAddAddress(builder, addressOffset)
	}
	if len(m.Items) > 0 {
		onlineboutique_flat.ShipOrderRequestAddItems(builder, itemsOffset)
	}
	return onlineboutique_flat.ShipOrderRequestEnd(builder), nil
}

// serializeCapnp serializes a proto message to Cap'n Proto format
func serializeCapnp(msg proto.Message) ([]byte, error) {
	msgCapnp, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		panic(fmt.Sprintf("failed to create Cap'n Proto message: %v", err))
	}

	switch m := msg.(type) {
	case *onlineboutique.Product:
		product, err := onlineboutique_capnp.NewRootProduct(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create Product: %v", err))
		}
		if err := serializeProductCapnp(product, m); err != nil {
			panic(fmt.Sprintf("failed to serialize Product: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.CartItem:
		cartItem, err := onlineboutique_capnp.NewRootCartItem(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create CartItem: %v", err))
		}
		if err := serializeCartItemCapnp(cartItem, m); err != nil {
			panic(fmt.Sprintf("failed to serialize CartItem: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.Money:
		money, err := onlineboutique_capnp.NewRootMoney(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create Money: %v", err))
		}
		if err := serializeMoneyCapnp(money, m); err != nil {
			panic(fmt.Sprintf("failed to serialize Money: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.Address:
		address, err := onlineboutique_capnp.NewRootAddress(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create Address: %v", err))
		}
		if err := serializeAddressCapnp(address, m); err != nil {
			panic(fmt.Sprintf("failed to serialize Address: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.GetProductRequest:
		req, err := onlineboutique_capnp.NewRootGetProductRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create GetProductRequest: %v", err))
		}
		if err := req.SetId(m.Id); err != nil {
			panic(fmt.Sprintf("failed to set GetProductRequest.Id: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.Empty:
		_, err := onlineboutique_capnp.NewRootEmpty(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create Empty: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.EmptyUser:
		emptyUser, err := onlineboutique_capnp.NewRootEmptyUser(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create EmptyUser: %v", err))
		}
		if err := emptyUser.SetUserId(m.UserId); err != nil {
			panic(fmt.Sprintf("failed to set EmptyUser.UserId: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.GetCartRequest:
		req, err := onlineboutique_capnp.NewRootGetCartRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create GetCartRequest: %v", err))
		}
		if err := req.SetUserId(m.UserId); err != nil {
			panic(fmt.Sprintf("failed to set GetCartRequest.UserId: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.EmptyCartRequest:
		req, err := onlineboutique_capnp.NewRootEmptyCartRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create EmptyCartRequest: %v", err))
		}
		if err := req.SetUserId(m.UserId); err != nil {
			panic(fmt.Sprintf("failed to set EmptyCartRequest.UserId: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.Ad:
		ad, err := onlineboutique_capnp.NewRootAd(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create Ad: %v", err))
		}
		if err := serializeAdCapnp(ad, m); err != nil {
			panic(fmt.Sprintf("failed to serialize Ad: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.AddItemRequest:
		req, err := onlineboutique_capnp.NewRootAddItemRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create AddItemRequest: %v", err))
		}
		if err := serializeAddItemRequestCapnp(req, m); err != nil {
			panic(fmt.Sprintf("failed to serialize AddItemRequest: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.AdRequest:
		req, err := onlineboutique_capnp.NewRootAdRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create AdRequest: %v", err))
		}
		if err := serializeAdRequestCapnp(req, m); err != nil {
			panic(fmt.Sprintf("failed to serialize AdRequest: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.AdResponse:
		resp, err := onlineboutique_capnp.NewRootAdResponse(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create AdResponse: %v", err))
		}
		if err := serializeAdResponseCapnp(resp, m); err != nil {
			panic(fmt.Sprintf("failed to serialize AdResponse: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.Cart:
		cart, err := onlineboutique_capnp.NewRootCart(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create Cart: %v", err))
		}
		if err := serializeCartCapnp(cart, m); err != nil {
			panic(fmt.Sprintf("failed to serialize Cart: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.ChargeRequest:
		req, err := onlineboutique_capnp.NewRootChargeRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create ChargeRequest: %v", err))
		}
		if err := serializeChargeRequestCapnp(req, m); err != nil {
			panic(fmt.Sprintf("failed to serialize ChargeRequest: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.ChargeResponse:
		resp, err := onlineboutique_capnp.NewRootChargeResponse(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create ChargeResponse: %v", err))
		}
		if err := resp.SetTransactionId(m.TransactionId); err != nil {
			panic(fmt.Sprintf("failed to set ChargeResponse.TransactionId: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.CreditCardInfo:
		info, err := onlineboutique_capnp.NewRootCreditCardInfo(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create CreditCardInfo: %v", err))
		}
		if err := serializeCreditCardInfoCapnp(info, m); err != nil {
			panic(fmt.Sprintf("failed to serialize CreditCardInfo: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.CurrencyConversionRequest:
		req, err := onlineboutique_capnp.NewRootCurrencyConversionRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create CurrencyConversionRequest: %v", err))
		}
		if err := serializeCurrencyConversionRequestCapnp(req, m); err != nil {
			panic(fmt.Sprintf("failed to serialize CurrencyConversionRequest: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.GetQuoteRequest:
		req, err := onlineboutique_capnp.NewRootGetQuoteRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create GetQuoteRequest: %v", err))
		}
		if err := serializeGetQuoteRequestCapnp(req, m); err != nil {
			panic(fmt.Sprintf("failed to serialize GetQuoteRequest: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.GetQuoteResponse:
		resp, err := onlineboutique_capnp.NewRootGetQuoteResponse(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create GetQuoteResponse: %v", err))
		}
		if err := serializeGetQuoteResponseCapnp(resp, m); err != nil {
			panic(fmt.Sprintf("failed to serialize GetQuoteResponse: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.GetSupportedCurrenciesResponse:
		resp, err := onlineboutique_capnp.NewRootGetSupportedCurrenciesResponse(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create GetSupportedCurrenciesResponse: %v", err))
		}
		if err := serializeGetSupportedCurrenciesResponseCapnp(resp, m); err != nil {
			panic(fmt.Sprintf("failed to serialize GetSupportedCurrenciesResponse: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.ListProductsResponse:
		resp, err := onlineboutique_capnp.NewRootListProductsResponse(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create ListProductsResponse: %v", err))
		}
		if err := serializeListProductsResponseCapnp(resp, m); err != nil {
			panic(fmt.Sprintf("failed to serialize ListProductsResponse: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.ListRecommendationsRequest:
		req, err := onlineboutique_capnp.NewRootListRecommendationsRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create ListRecommendationsRequest: %v", err))
		}
		if err := serializeListRecommendationsRequestCapnp(req, m); err != nil {
			panic(fmt.Sprintf("failed to serialize ListRecommendationsRequest: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.ListRecommendationsResponse:
		resp, err := onlineboutique_capnp.NewRootListRecommendationsResponse(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create ListRecommendationsResponse: %v", err))
		}
		if err := serializeListRecommendationsResponseCapnp(resp, m); err != nil {
			panic(fmt.Sprintf("failed to serialize ListRecommendationsResponse: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.OrderItem:
		item, err := onlineboutique_capnp.NewRootOrderItem(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create OrderItem: %v", err))
		}
		if err := serializeOrderItemCapnp(item, m); err != nil {
			panic(fmt.Sprintf("failed to serialize OrderItem: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.OrderResult:
		result, err := onlineboutique_capnp.NewRootOrderResult(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create OrderResult: %v", err))
		}
		if err := serializeOrderResultCapnp(result, m); err != nil {
			panic(fmt.Sprintf("failed to serialize OrderResult: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.PlaceOrderRequest:
		req, err := onlineboutique_capnp.NewRootPlaceOrderRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create PlaceOrderRequest: %v", err))
		}
		if err := serializePlaceOrderRequestCapnp(req, m); err != nil {
			panic(fmt.Sprintf("failed to serialize PlaceOrderRequest: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.PlaceOrderResponse:
		resp, err := onlineboutique_capnp.NewRootPlaceOrderResponse(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create PlaceOrderResponse: %v", err))
		}
		if err := serializePlaceOrderResponseCapnp(resp, m); err != nil {
			panic(fmt.Sprintf("failed to serialize PlaceOrderResponse: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.SearchProductsResponse:
		resp, err := onlineboutique_capnp.NewRootSearchProductsResponse(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create SearchProductsResponse: %v", err))
		}
		if err := serializeSearchProductsResponseCapnp(resp, m); err != nil {
			panic(fmt.Sprintf("failed to serialize SearchProductsResponse: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.SendOrderConfirmationRequest:
		req, err := onlineboutique_capnp.NewRootSendOrderConfirmationRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create SendOrderConfirmationRequest: %v", err))
		}
		if err := serializeSendOrderConfirmationRequestCapnp(req, m); err != nil {
			panic(fmt.Sprintf("failed to serialize SendOrderConfirmationRequest: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.ShipOrderRequest:
		req, err := onlineboutique_capnp.NewRootShipOrderRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create ShipOrderRequest: %v", err))
		}
		if err := serializeShipOrderRequestCapnp(req, m); err != nil {
			panic(fmt.Sprintf("failed to serialize ShipOrderRequest: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.ShipOrderResponse:
		resp, err := onlineboutique_capnp.NewRootShipOrderResponse(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create ShipOrderResponse: %v", err))
		}
		if err := resp.SetTrackingId(m.TrackingId); err != nil {
			panic(fmt.Sprintf("failed to set ShipOrderResponse.TrackingId: %v", err))
		}
		return msgCapnp.Marshal()
	case *onlineboutique.SearchProductsRequest:
		req, err := onlineboutique_capnp.NewRootSearchProductsRequest(seg)
		if err != nil {
			panic(fmt.Sprintf("failed to create SearchProductsRequest: %v", err))
		}
		if err := req.SetQuery(m.Query); err != nil {
			panic(fmt.Sprintf("failed to set SearchProductsRequest.Query: %v", err))
		}
		return msgCapnp.Marshal()
	default:
		panic(fmt.Sprintf("unsupported message type for Cap'n Proto: %T", msg))
	}
}

// serializeProductCapnp serializes a Product message to Cap'n Proto
func serializeProductCapnp(product onlineboutique_capnp.Product, p *onlineboutique.Product) error {
	if err := product.SetId(p.Id); err != nil {
		return err
	}
	if err := product.SetName(p.Name); err != nil {
		return err
	}
	if err := product.SetDescription(p.Description); err != nil {
		return err
	}
	if err := product.SetPicture(p.Picture); err != nil {
		return err
	}
	if p.PriceUsd != nil {
		money, err := product.NewPriceUsd()
		if err != nil {
			return err
		}
		if err := serializeMoneyCapnp(money, p.PriceUsd); err != nil {
			return err
		}
	}
	if len(p.Categories) > 0 {
		categories, err := product.NewCategories(int32(len(p.Categories)))
		if err != nil {
			return err
		}
		for i, cat := range p.Categories {
			if err := categories.Set(i, cat); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializeCartItemCapnp serializes a CartItem message to Cap'n Proto
func serializeCartItemCapnp(cartItem onlineboutique_capnp.CartItem, c *onlineboutique.CartItem) error {
	if err := cartItem.SetProductId(c.ProductId); err != nil {
		return err
	}
	cartItem.SetQuantity(c.Quantity)
	return nil
}

// serializeMoneyCapnp serializes a Money message to Cap'n Proto
func serializeMoneyCapnp(money onlineboutique_capnp.Money, m *onlineboutique.Money) error {
	if err := money.SetCurrencyCode(m.CurrencyCode); err != nil {
		return err
	}
	money.SetUnits(m.Units)
	money.SetNanos(m.Nanos)
	return nil
}

// serializeAddressCapnp serializes an Address message to Cap'n Proto
func serializeAddressCapnp(address onlineboutique_capnp.Address, a *onlineboutique.Address) error {
	if err := address.SetStreetAddress(a.StreetAddress); err != nil {
		return err
	}
	if err := address.SetCity(a.City); err != nil {
		return err
	}
	if err := address.SetState(a.State); err != nil {
		return err
	}
	if err := address.SetCountry(a.Country); err != nil {
		return err
	}
	address.SetZipCode(a.ZipCode)
	return nil
}

// serializeAdCapnp serializes an Ad message to Cap'n Proto
func serializeAdCapnp(ad onlineboutique_capnp.Ad, a *onlineboutique.Ad) error {
	if err := ad.SetRedirectUrl(a.RedirectUrl); err != nil {
		return err
	}
	if err := ad.SetText(a.Text); err != nil {
		return err
	}
	return nil
}

// serializeAddItemRequestCapnp serializes an AddItemRequest message to Cap'n Proto
func serializeAddItemRequestCapnp(req onlineboutique_capnp.AddItemRequest, m *onlineboutique.AddItemRequest) error {
	if err := req.SetUserId(m.UserId); err != nil {
		return err
	}
	if m.Item != nil {
		item, err := req.NewItem()
		if err != nil {
			return err
		}
		if err := serializeCartItemCapnp(item, m.Item); err != nil {
			return err
		}
	}
	return nil
}

// serializeAdRequestCapnp serializes an AdRequest message to Cap'n Proto
func serializeAdRequestCapnp(req onlineboutique_capnp.AdRequest, m *onlineboutique.AdRequest) error {
	if err := req.SetUserId(m.UserId); err != nil {
		return err
	}
	if len(m.ContextKeys) > 0 {
		contextKeys, err := req.NewContextKeys(int32(len(m.ContextKeys)))
		if err != nil {
			return err
		}
		for i, key := range m.ContextKeys {
			if err := contextKeys.Set(i, key); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializeAdResponseCapnp serializes an AdResponse message to Cap'n Proto
func serializeAdResponseCapnp(resp onlineboutique_capnp.AdResponse, m *onlineboutique.AdResponse) error {
	if len(m.Ads) > 0 {
		ads, err := resp.NewAds(int32(len(m.Ads)))
		if err != nil {
			return err
		}
		for i := range m.Ads {
			adCapnp := ads.At(i)
			if err := serializeAdCapnp(adCapnp, m.Ads[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializeCartCapnp serializes a Cart message to Cap'n Proto
func serializeCartCapnp(cart onlineboutique_capnp.Cart, m *onlineboutique.Cart) error {
	if err := cart.SetUserId(m.UserId); err != nil {
		return err
	}
	if len(m.Items) > 0 {
		items, err := cart.NewItems(int32(len(m.Items)))
		if err != nil {
			return err
		}
		for i := range m.Items {
			itemCapnp := items.At(i)
			if err := serializeCartItemCapnp(itemCapnp, m.Items[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializeCreditCardInfoCapnp serializes a CreditCardInfo message to Cap'n Proto
func serializeCreditCardInfoCapnp(info onlineboutique_capnp.CreditCardInfo, m *onlineboutique.CreditCardInfo) error {
	if err := info.SetCreditCardNumber(m.CreditCardNumber); err != nil {
		return err
	}
	info.SetCreditCardCvv(m.CreditCardCvv)
	info.SetCreditCardExpirationYear(m.CreditCardExpirationYear)
	info.SetCreditCardExpirationMonth(m.CreditCardExpirationMonth)
	return nil
}

// serializeChargeRequestCapnp serializes a ChargeRequest message to Cap'n Proto
func serializeChargeRequestCapnp(req onlineboutique_capnp.ChargeRequest, m *onlineboutique.ChargeRequest) error {
	if m.Amount != nil {
		amount, err := req.NewAmount()
		if err != nil {
			return err
		}
		if err := serializeMoneyCapnp(amount, m.Amount); err != nil {
			return err
		}
	}
	if m.CreditCard != nil {
		card, err := req.NewCreditCard()
		if err != nil {
			return err
		}
		if err := serializeCreditCardInfoCapnp(card, m.CreditCard); err != nil {
			return err
		}
	}
	return nil
}

// serializeCurrencyConversionRequestCapnp serializes a CurrencyConversionRequest message to Cap'n Proto
func serializeCurrencyConversionRequestCapnp(req onlineboutique_capnp.CurrencyConversionRequest, m *onlineboutique.CurrencyConversionRequest) error {
	if m.From != nil {
		from, err := req.NewFrom()
		if err != nil {
			return err
		}
		if err := serializeMoneyCapnp(from, m.From); err != nil {
			return err
		}
	}
	if err := req.SetToCode(m.ToCode); err != nil {
		return err
	}
	if err := req.SetUserId(m.UserId); err != nil {
		return err
	}
	return nil
}

// serializeGetQuoteRequestCapnp serializes a GetQuoteRequest message to Cap'n Proto
func serializeGetQuoteRequestCapnp(req onlineboutique_capnp.GetQuoteRequest, m *onlineboutique.GetQuoteRequest) error {
	if m.Address != nil {
		addr, err := req.NewAddress()
		if err != nil {
			return err
		}
		if err := serializeAddressCapnp(addr, m.Address); err != nil {
			return err
		}
	}
	if len(m.Items) > 0 {
		items, err := req.NewItems(int32(len(m.Items)))
		if err != nil {
			return err
		}
		for i := range m.Items {
			itemCapnp := items.At(i)
			if err := serializeCartItemCapnp(itemCapnp, m.Items[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializeGetQuoteResponseCapnp serializes a GetQuoteResponse message to Cap'n Proto
func serializeGetQuoteResponseCapnp(resp onlineboutique_capnp.GetQuoteResponse, m *onlineboutique.GetQuoteResponse) error {
	if m.CostUsd != nil {
		cost, err := resp.NewCostUsd()
		if err != nil {
			return err
		}
		if err := serializeMoneyCapnp(cost, m.CostUsd); err != nil {
			return err
		}
	}
	return nil
}

// serializeGetSupportedCurrenciesResponseCapnp serializes a GetSupportedCurrenciesResponse message to Cap'n Proto
func serializeGetSupportedCurrenciesResponseCapnp(resp onlineboutique_capnp.GetSupportedCurrenciesResponse, m *onlineboutique.GetSupportedCurrenciesResponse) error {
	if len(m.CurrencyCodes) > 0 {
		codes, err := resp.NewCurrencyCodes(int32(len(m.CurrencyCodes)))
		if err != nil {
			return err
		}
		for i, code := range m.CurrencyCodes {
			if err := codes.Set(i, code); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializeListProductsResponseCapnp serializes a ListProductsResponse message to Cap'n Proto
func serializeListProductsResponseCapnp(resp onlineboutique_capnp.ListProductsResponse, m *onlineboutique.ListProductsResponse) error {
	if len(m.Products) > 0 {
		products, err := resp.NewProducts(int32(len(m.Products)))
		if err != nil {
			return err
		}
		for i := range m.Products {
			productCapnp := products.At(i)
			if err := serializeProductCapnp(productCapnp, m.Products[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializeListRecommendationsRequestCapnp serializes a ListRecommendationsRequest message to Cap'n Proto
func serializeListRecommendationsRequestCapnp(req onlineboutique_capnp.ListRecommendationsRequest, m *onlineboutique.ListRecommendationsRequest) error {
	if err := req.SetUserId(m.UserId); err != nil {
		return err
	}
	if len(m.ProductIds) > 0 {
		productIds, err := req.NewProductIds(int32(len(m.ProductIds)))
		if err != nil {
			return err
		}
		for i, id := range m.ProductIds {
			if err := productIds.Set(i, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializeListRecommendationsResponseCapnp serializes a ListRecommendationsResponse message to Cap'n Proto
func serializeListRecommendationsResponseCapnp(resp onlineboutique_capnp.ListRecommendationsResponse, m *onlineboutique.ListRecommendationsResponse) error {
	if len(m.ProductIds) > 0 {
		productIds, err := resp.NewProductIds(int32(len(m.ProductIds)))
		if err != nil {
			return err
		}
		for i, id := range m.ProductIds {
			if err := productIds.Set(i, id); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializeOrderItemCapnp serializes an OrderItem message to Cap'n Proto
func serializeOrderItemCapnp(item onlineboutique_capnp.OrderItem, m *onlineboutique.OrderItem) error {
	if m.Item != nil {
		itemCapnp, err := item.NewItem()
		if err != nil {
			return err
		}
		if err := serializeCartItemCapnp(itemCapnp, m.Item); err != nil {
			return err
		}
	}
	if m.Cost != nil {
		cost, err := item.NewCost()
		if err != nil {
			return err
		}
		if err := serializeMoneyCapnp(cost, m.Cost); err != nil {
			return err
		}
	}
	return nil
}

// serializeOrderResultCapnp serializes an OrderResult message to Cap'n Proto
func serializeOrderResultCapnp(result onlineboutique_capnp.OrderResult, m *onlineboutique.OrderResult) error {
	if err := result.SetOrderId(m.OrderId); err != nil {
		return err
	}
	if err := result.SetShippingTrackingId(m.ShippingTrackingId); err != nil {
		return err
	}
	if m.ShippingCost != nil {
		cost, err := result.NewShippingCost()
		if err != nil {
			return err
		}
		if err := serializeMoneyCapnp(cost, m.ShippingCost); err != nil {
			return err
		}
	}
	if m.ShippingAddress != nil {
		addr, err := result.NewShippingAddress()
		if err != nil {
			return err
		}
		if err := serializeAddressCapnp(addr, m.ShippingAddress); err != nil {
			return err
		}
	}
	if len(m.Items) > 0 {
		items, err := result.NewItems(int32(len(m.Items)))
		if err != nil {
			return err
		}
		for i := range m.Items {
			itemCapnp := items.At(i)
			if err := serializeOrderItemCapnp(itemCapnp, m.Items[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializePlaceOrderRequestCapnp serializes a PlaceOrderRequest message to Cap'n Proto
func serializePlaceOrderRequestCapnp(req onlineboutique_capnp.PlaceOrderRequest, m *onlineboutique.PlaceOrderRequest) error {
	if err := req.SetUserId(m.UserId); err != nil {
		return err
	}
	if err := req.SetUserCurrency(m.UserCurrency); err != nil {
		return err
	}
	if m.Address != nil {
		addr, err := req.NewAddress()
		if err != nil {
			return err
		}
		if err := serializeAddressCapnp(addr, m.Address); err != nil {
			return err
		}
	}
	if err := req.SetEmail(m.Email); err != nil {
		return err
	}
	if m.CreditCard != nil {
		card, err := req.NewCreditCard()
		if err != nil {
			return err
		}
		if err := serializeCreditCardInfoCapnp(card, m.CreditCard); err != nil {
			return err
		}
	}
	return nil
}

// serializePlaceOrderResponseCapnp serializes a PlaceOrderResponse message to Cap'n Proto
func serializePlaceOrderResponseCapnp(resp onlineboutique_capnp.PlaceOrderResponse, m *onlineboutique.PlaceOrderResponse) error {
	if m.Order != nil {
		order, err := resp.NewOrder()
		if err != nil {
			return err
		}
		if err := serializeOrderResultCapnp(order, m.Order); err != nil {
			return err
		}
	}
	return nil
}

// serializeSearchProductsResponseCapnp serializes a SearchProductsResponse message to Cap'n Proto
func serializeSearchProductsResponseCapnp(resp onlineboutique_capnp.SearchProductsResponse, m *onlineboutique.SearchProductsResponse) error {
	if len(m.Results) > 0 {
		results, err := resp.NewResults(int32(len(m.Results)))
		if err != nil {
			return err
		}
		for i := range m.Results {
			productCapnp := results.At(i)
			if err := serializeProductCapnp(productCapnp, m.Results[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// serializeSendOrderConfirmationRequestCapnp serializes a SendOrderConfirmationRequest message to Cap'n Proto
func serializeSendOrderConfirmationRequestCapnp(req onlineboutique_capnp.SendOrderConfirmationRequest, m *onlineboutique.SendOrderConfirmationRequest) error {
	if err := req.SetEmail(m.Email); err != nil {
		return err
	}
	if m.Order != nil {
		order, err := req.NewOrder()
		if err != nil {
			return err
		}
		if err := serializeOrderResultCapnp(order, m.Order); err != nil {
			return err
		}
	}
	return nil
}

// serializeShipOrderRequestCapnp serializes a ShipOrderRequest message to Cap'n Proto
func serializeShipOrderRequestCapnp(req onlineboutique_capnp.ShipOrderRequest, m *onlineboutique.ShipOrderRequest) error {
	if m.Address != nil {
		addr, err := req.NewAddress()
		if err != nil {
			return err
		}
		if err := serializeAddressCapnp(addr, m.Address); err != nil {
			return err
		}
	}
	if len(m.Items) > 0 {
		items, err := req.NewItems(int32(len(m.Items)))
		if err != nil {
			return err
		}
		for i := range m.Items {
			itemCapnp := items.At(i)
			if err := serializeCartItemCapnp(itemCapnp, m.Items[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

// unmarshalSymphony unmarshals Symphony format data into a proto message
func unmarshalSymphony(msg proto.Message, data []byte) error {
	switch m := msg.(type) {
	case *onlineboutique.CartItem:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.AddItemRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.EmptyCartRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.GetCartRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.Cart:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.Empty:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.EmptyUser:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.ListRecommendationsRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.ListRecommendationsResponse:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.Product:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.ListProductsResponse:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.GetProductRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.SearchProductsRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.SearchProductsResponse:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.GetQuoteRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.GetQuoteResponse:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.ShipOrderRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.ShipOrderResponse:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.Address:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.Money:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.GetSupportedCurrenciesResponse:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.CurrencyConversionRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.CreditCardInfo:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.ChargeRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.ChargeResponse:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.OrderItem:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.OrderResult:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.SendOrderConfirmationRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.PlaceOrderRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.PlaceOrderResponse:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.AdRequest:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.AdResponse:
		return m.UnmarshalSymphony(data)
	case *onlineboutique.Ad:
		return m.UnmarshalSymphony(data)
	default:
		panic(fmt.Sprintf("unsupported message type for Symphony: %T", msg))
	}
}

// unmarshalSymphonyHybrid unmarshals Symphony Hybrid format data into a proto message
func unmarshalSymphonyHybrid(msg proto.Message, data []byte) error {
	switch m := msg.(type) {
	case *onlineboutique.CartItem:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.AddItemRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.EmptyCartRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.GetCartRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.Cart:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.Empty:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.EmptyUser:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.ListRecommendationsRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.ListRecommendationsResponse:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.Product:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.ListProductsResponse:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.GetProductRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.SearchProductsRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.SearchProductsResponse:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.GetQuoteRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.GetQuoteResponse:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.ShipOrderRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.ShipOrderResponse:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.Address:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.Money:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.GetSupportedCurrenciesResponse:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.CurrencyConversionRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.CreditCardInfo:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.ChargeRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.ChargeResponse:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.OrderItem:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.OrderResult:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.SendOrderConfirmationRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.PlaceOrderRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.PlaceOrderResponse:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.AdRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.AdResponse:
		return m.UnmarshalSymphonyHybrid(data)
	case *onlineboutique.Ad:
		return m.UnmarshalSymphonyHybrid(data)
	default:
		panic(fmt.Sprintf("unsupported message type for Symphony Hybrid: %T", msg))
	}
}

// accessAllFields accesses all fields of a proto message to ensure deserialization
func accessAllFields(msg proto.Message) {
	switch m := msg.(type) {
	case *onlineboutique.CartItem:
		_ = m.GetProductId()
		_ = m.GetQuantity()
	case *onlineboutique.AddItemRequest:
		_ = m.GetUserId()
		if item := m.GetItem(); item != nil {
			accessAllFields(item)
		}
	case *onlineboutique.EmptyCartRequest:
		_ = m.GetUserId()
	case *onlineboutique.GetCartRequest:
		_ = m.GetUserId()
	case *onlineboutique.Cart:
		_ = m.GetUserId()
		for _, item := range m.GetItems() {
			accessAllFields(item)
		}
	case *onlineboutique.Empty:
		// Empty message has no fields
	case *onlineboutique.EmptyUser:
		_ = m.GetUserId()
	case *onlineboutique.ListRecommendationsRequest:
		_ = m.GetUserId()
		_ = m.GetProductIds()
	case *onlineboutique.ListRecommendationsResponse:
		_ = m.GetProductIds()
	case *onlineboutique.Product:
		_ = m.GetId()
		_ = m.GetName()
		_ = m.GetDescription()
		_ = m.GetPicture()
		if price := m.GetPriceUsd(); price != nil {
			accessAllFields(price)
		}
		_ = m.GetCategories()
	case *onlineboutique.ListProductsResponse:
		for _, product := range m.GetProducts() {
			accessAllFields(product)
		}
	case *onlineboutique.GetProductRequest:
		_ = m.GetId()
	case *onlineboutique.SearchProductsRequest:
		_ = m.GetQuery()
	case *onlineboutique.SearchProductsResponse:
		for _, product := range m.GetResults() {
			accessAllFields(product)
		}
	case *onlineboutique.GetQuoteRequest:
		if addr := m.GetAddress(); addr != nil {
			accessAllFields(addr)
		}
		for _, item := range m.GetItems() {
			accessAllFields(item)
		}
	case *onlineboutique.GetQuoteResponse:
		if cost := m.GetCostUsd(); cost != nil {
			accessAllFields(cost)
		}
	case *onlineboutique.ShipOrderRequest:
		if addr := m.GetAddress(); addr != nil {
			accessAllFields(addr)
		}
		for _, item := range m.GetItems() {
			accessAllFields(item)
		}
	case *onlineboutique.ShipOrderResponse:
		_ = m.GetTrackingId()
	case *onlineboutique.Address:
		_ = m.GetStreetAddress()
		_ = m.GetCity()
		_ = m.GetState()
		_ = m.GetCountry()
		_ = m.GetZipCode()
	case *onlineboutique.Money:
		_ = m.GetCurrencyCode()
		_ = m.GetUnits()
		_ = m.GetNanos()
	case *onlineboutique.GetSupportedCurrenciesResponse:
		_ = m.GetCurrencyCodes()
	case *onlineboutique.CurrencyConversionRequest:
		if from := m.GetFrom(); from != nil {
			accessAllFields(from)
		}
		_ = m.GetToCode()
		_ = m.GetUserId()
	case *onlineboutique.CreditCardInfo:
		_ = m.GetCreditCardNumber()
		_ = m.GetCreditCardCvv()
		_ = m.GetCreditCardExpirationYear()
		_ = m.GetCreditCardExpirationMonth()
	case *onlineboutique.ChargeRequest:
		if amount := m.GetAmount(); amount != nil {
			accessAllFields(amount)
		}
		if card := m.GetCreditCard(); card != nil {
			accessAllFields(card)
		}
	case *onlineboutique.ChargeResponse:
		_ = m.GetTransactionId()
	case *onlineboutique.OrderItem:
		if item := m.GetItem(); item != nil {
			accessAllFields(item)
		}
		if cost := m.GetCost(); cost != nil {
			accessAllFields(cost)
		}
	case *onlineboutique.OrderResult:
		_ = m.GetOrderId()
		_ = m.GetShippingTrackingId()
		if cost := m.GetShippingCost(); cost != nil {
			accessAllFields(cost)
		}
		if addr := m.GetShippingAddress(); addr != nil {
			accessAllFields(addr)
		}
		for _, item := range m.GetItems() {
			accessAllFields(item)
		}
	case *onlineboutique.SendOrderConfirmationRequest:
		_ = m.GetEmail()
		if order := m.GetOrder(); order != nil {
			accessAllFields(order)
		}
	case *onlineboutique.PlaceOrderRequest:
		_ = m.GetUserId()
		_ = m.GetUserCurrency()
		if addr := m.GetAddress(); addr != nil {
			accessAllFields(addr)
		}
		_ = m.GetEmail()
		if card := m.GetCreditCard(); card != nil {
			accessAllFields(card)
		}
	case *onlineboutique.PlaceOrderResponse:
		if order := m.GetOrder(); order != nil {
			accessAllFields(order)
		}
	case *onlineboutique.AdRequest:
		_ = m.GetUserId()
		_ = m.GetContextKeys()
	case *onlineboutique.AdResponse:
		for _, ad := range m.GetAds() {
			accessAllFields(ad)
		}
	case *onlineboutique.Ad:
		_ = m.GetRedirectUrl()
		_ = m.GetText()
	default:
		panic(fmt.Errorf("unsupported message type for accessAllFields: %T", m))
	}
}

// unmarshalFlatbuffersAndAccessFields unmarshals a FlatBuffers buffer and accesses all fields
func unmarshalFlatbuffersAndAccessFields(typeName string, data []byte) error {
	switch typeName {
	case "Product":
		obj := onlineboutique_flat.GetRootAsProduct(data, 0)
		_ = string(obj.Id())
		_ = string(obj.Name())
		_ = string(obj.Description())
		_ = string(obj.Picture())
		if price := obj.PriceUsd(nil); price != nil {
			_ = string(price.CurrencyCode())
			_ = price.Units()
			_ = price.Nanos()
		}
		for j := 0; j < obj.CategoriesLength(); j++ {
			_ = string(obj.Categories(j))
		}
	case "CartItem":
		obj := onlineboutique_flat.GetRootAsCartItem(data, 0)
		_ = string(obj.ProductId())
		_ = obj.Quantity()
	case "Money":
		obj := onlineboutique_flat.GetRootAsMoney(data, 0)
		_ = string(obj.CurrencyCode())
		_ = obj.Units()
		_ = obj.Nanos()
	case "Address":
		obj := onlineboutique_flat.GetRootAsAddress(data, 0)
		_ = string(obj.StreetAddress())
		_ = string(obj.City())
		_ = string(obj.State())
		_ = string(obj.Country())
		_ = obj.ZipCode()
	case "GetProductRequest":
		obj := onlineboutique_flat.GetRootAsGetProductRequest(data, 0)
		_ = string(obj.Id())
	case "SearchProductsRequest":
		obj := onlineboutique_flat.GetRootAsSearchProductsRequest(data, 0)
		_ = string(obj.Query())
	case "Empty":
		_ = onlineboutique_flat.GetRootAsEmpty(data, 0)
	case "EmptyUser":
		obj := onlineboutique_flat.GetRootAsEmptyUser(data, 0)
		_ = string(obj.UserId())
	case "GetCartRequest":
		obj := onlineboutique_flat.GetRootAsGetCartRequest(data, 0)
		_ = string(obj.UserId())
	case "EmptyCartRequest":
		obj := onlineboutique_flat.GetRootAsEmptyCartRequest(data, 0)
		_ = string(obj.UserId())
	case "Cart":
		obj := onlineboutique_flat.GetRootAsCart(data, 0)
		_ = string(obj.UserId())
		item := &onlineboutique_flat.CartItem{}
		for j := 0; j < obj.ItemsLength(); j++ {
			if obj.Items(item, j) {
				_ = string(item.ProductId())
				_ = item.Quantity()
			}
		}
	case "AddItemRequest":
		obj := onlineboutique_flat.GetRootAsAddItemRequest(data, 0)
		_ = string(obj.UserId())
		if item := obj.Item(nil); item != nil {
			_ = string(item.ProductId())
			_ = item.Quantity()
		}
	case "GetQuoteRequest":
		obj := onlineboutique_flat.GetRootAsGetQuoteRequest(data, 0)
		if addr := obj.Address(nil); addr != nil {
			_ = string(addr.StreetAddress())
			_ = string(addr.City())
			_ = string(addr.State())
			_ = string(addr.Country())
			_ = addr.ZipCode()
		}
		item := &onlineboutique_flat.CartItem{}
		for j := 0; j < obj.ItemsLength(); j++ {
			if obj.Items(item, j) {
				_ = string(item.ProductId())
				_ = item.Quantity()
			}
		}
	case "GetQuoteResponse":
		obj := onlineboutique_flat.GetRootAsGetQuoteResponse(data, 0)
		if cost := obj.CostUsd(nil); cost != nil {
			_ = string(cost.CurrencyCode())
			_ = cost.Units()
			_ = cost.Nanos()
		}
	case "ListProductsResponse":
		obj := onlineboutique_flat.GetRootAsListProductsResponse(data, 0)
		product := &onlineboutique_flat.Product{}
		for j := 0; j < obj.ProductsLength(); j++ {
			if obj.Products(product, j) {
				_ = string(product.Id())
				_ = string(product.Name())
				_ = string(product.Description())
				_ = string(product.Picture())
				if price := product.PriceUsd(nil); price != nil {
					_ = string(price.CurrencyCode())
					_ = price.Units()
					_ = price.Nanos()
				}
				for k := 0; k < product.CategoriesLength(); k++ {
					_ = string(product.Categories(k))
				}
			}
		}
	case "SearchProductsResponse":
		obj := onlineboutique_flat.GetRootAsSearchProductsResponse(data, 0)
		product := &onlineboutique_flat.Product{}
		for j := 0; j < obj.ResultsLength(); j++ {
			if obj.Results(product, j) {
				_ = string(product.Id())
				_ = string(product.Name())
				_ = string(product.Description())
				_ = string(product.Picture())
				if price := product.PriceUsd(nil); price != nil {
					_ = string(price.CurrencyCode())
					_ = price.Units()
					_ = price.Nanos()
				}
				for k := 0; k < product.CategoriesLength(); k++ {
					_ = string(product.Categories(k))
				}
			}
		}
	case "Ad":
		obj := onlineboutique_flat.GetRootAsAd(data, 0)
		_ = string(obj.RedirectUrl())
		_ = string(obj.Text())
	case "AdRequest":
		obj := onlineboutique_flat.GetRootAsAdRequest(data, 0)
		_ = string(obj.UserId())
		for j := 0; j < obj.ContextKeysLength(); j++ {
			_ = string(obj.ContextKeys(j))
		}
	case "AdResponse":
		obj := onlineboutique_flat.GetRootAsAdResponse(data, 0)
		ad := &onlineboutique_flat.Ad{}
		for j := 0; j < obj.AdsLength(); j++ {
			if obj.Ads(ad, j) {
				_ = string(ad.RedirectUrl())
				_ = string(ad.Text())
			}
		}
	case "ChargeRequest":
		obj := onlineboutique_flat.GetRootAsChargeRequest(data, 0)
		if amount := obj.Amount(nil); amount != nil {
			_ = string(amount.CurrencyCode())
			_ = amount.Units()
			_ = amount.Nanos()
		}
		if card := obj.CreditCard(nil); card != nil {
			_ = string(card.CreditCardNumber())
			_ = card.CreditCardCvv()
			_ = card.CreditCardExpirationYear()
			_ = card.CreditCardExpirationMonth()
		}
	case "ChargeResponse":
		obj := onlineboutique_flat.GetRootAsChargeResponse(data, 0)
		_ = string(obj.TransactionId())
	case "CreditCardInfo":
		obj := onlineboutique_flat.GetRootAsCreditCardInfo(data, 0)
		_ = string(obj.CreditCardNumber())
		_ = obj.CreditCardCvv()
		_ = obj.CreditCardExpirationYear()
		_ = obj.CreditCardExpirationMonth()
	case "CurrencyConversionRequest":
		obj := onlineboutique_flat.GetRootAsCurrencyConversionRequest(data, 0)
		if from := obj.From(nil); from != nil {
			_ = string(from.CurrencyCode())
			_ = from.Units()
			_ = from.Nanos()
		}
		_ = string(obj.ToCode())
		_ = string(obj.UserId())
	case "GetSupportedCurrenciesResponse":
		obj := onlineboutique_flat.GetRootAsGetSupportedCurrenciesResponse(data, 0)
		for j := 0; j < obj.CurrencyCodesLength(); j++ {
			_ = string(obj.CurrencyCodes(j))
		}
	case "ListRecommendationsRequest":
		obj := onlineboutique_flat.GetRootAsListRecommendationsRequest(data, 0)
		_ = string(obj.UserId())
		for j := 0; j < obj.ProductIdsLength(); j++ {
			_ = string(obj.ProductIds(j))
		}
	case "ListRecommendationsResponse":
		obj := onlineboutique_flat.GetRootAsListRecommendationsResponse(data, 0)
		for j := 0; j < obj.ProductIdsLength(); j++ {
			_ = string(obj.ProductIds(j))
		}
	case "OrderItem":
		obj := onlineboutique_flat.GetRootAsOrderItem(data, 0)
		if item := obj.Item(nil); item != nil {
			_ = string(item.ProductId())
			_ = item.Quantity()
		}
		if cost := obj.Cost(nil); cost != nil {
			_ = string(cost.CurrencyCode())
			_ = cost.Units()
			_ = cost.Nanos()
		}
	case "OrderResult":
		obj := onlineboutique_flat.GetRootAsOrderResult(data, 0)
		_ = string(obj.OrderId())
		_ = string(obj.ShippingTrackingId())
		if cost := obj.ShippingCost(nil); cost != nil {
			_ = string(cost.CurrencyCode())
			_ = cost.Units()
			_ = cost.Nanos()
		}
		if addr := obj.ShippingAddress(nil); addr != nil {
			_ = string(addr.StreetAddress())
			_ = string(addr.City())
			_ = string(addr.State())
			_ = string(addr.Country())
			_ = addr.ZipCode()
		}
		orderItem := &onlineboutique_flat.OrderItem{}
		for j := 0; j < obj.ItemsLength(); j++ {
			if obj.Items(orderItem, j) {
				if item := orderItem.Item(nil); item != nil {
					_ = string(item.ProductId())
					_ = item.Quantity()
				}
				if cost := orderItem.Cost(nil); cost != nil {
					_ = string(cost.CurrencyCode())
					_ = cost.Units()
					_ = cost.Nanos()
				}
			}
		}
	case "PlaceOrderRequest":
		obj := onlineboutique_flat.GetRootAsPlaceOrderRequest(data, 0)
		_ = string(obj.UserId())
		_ = string(obj.UserCurrency())
		if addr := obj.Address(nil); addr != nil {
			_ = string(addr.StreetAddress())
			_ = string(addr.City())
			_ = string(addr.State())
			_ = string(addr.Country())
			_ = addr.ZipCode()
		}
		_ = string(obj.Email())
		if card := obj.CreditCard(nil); card != nil {
			_ = string(card.CreditCardNumber())
			_ = card.CreditCardCvv()
			_ = card.CreditCardExpirationYear()
			_ = card.CreditCardExpirationMonth()
		}
	case "PlaceOrderResponse":
		obj := onlineboutique_flat.GetRootAsPlaceOrderResponse(data, 0)
		if order := obj.Order(nil); order != nil {
			_ = string(order.OrderId())
			_ = string(order.ShippingTrackingId())
			if cost := order.ShippingCost(nil); cost != nil {
				_ = string(cost.CurrencyCode())
				_ = cost.Units()
				_ = cost.Nanos()
			}
			if addr := order.ShippingAddress(nil); addr != nil {
				_ = string(addr.StreetAddress())
				_ = string(addr.City())
				_ = string(addr.State())
				_ = string(addr.Country())
				_ = addr.ZipCode()
			}
			orderItem := &onlineboutique_flat.OrderItem{}
			for j := 0; j < order.ItemsLength(); j++ {
				if order.Items(orderItem, j) {
					if item := orderItem.Item(nil); item != nil {
						_ = string(item.ProductId())
						_ = item.Quantity()
					}
					if cost := orderItem.Cost(nil); cost != nil {
						_ = string(cost.CurrencyCode())
						_ = cost.Units()
						_ = cost.Nanos()
					}
				}
			}
		}
	case "SendOrderConfirmationRequest":
		obj := onlineboutique_flat.GetRootAsSendOrderConfirmationRequest(data, 0)
		_ = string(obj.Email())
		if order := obj.Order(nil); order != nil {
			_ = string(order.OrderId())
			_ = string(order.ShippingTrackingId())
			if cost := order.ShippingCost(nil); cost != nil {
				_ = string(cost.CurrencyCode())
				_ = cost.Units()
				_ = cost.Nanos()
			}
			if addr := order.ShippingAddress(nil); addr != nil {
				_ = string(addr.StreetAddress())
				_ = string(addr.City())
				_ = string(addr.State())
				_ = string(addr.Country())
				_ = addr.ZipCode()
			}
			orderItem := &onlineboutique_flat.OrderItem{}
			for j := 0; j < order.ItemsLength(); j++ {
				if order.Items(orderItem, j) {
					if item := orderItem.Item(nil); item != nil {
						_ = string(item.ProductId())
						_ = item.Quantity()
					}
					if cost := orderItem.Cost(nil); cost != nil {
						_ = string(cost.CurrencyCode())
						_ = cost.Units()
						_ = cost.Nanos()
					}
				}
			}
		}
	case "ShipOrderRequest":
		obj := onlineboutique_flat.GetRootAsShipOrderRequest(data, 0)
		if addr := obj.Address(nil); addr != nil {
			_ = string(addr.StreetAddress())
			_ = string(addr.City())
			_ = string(addr.State())
			_ = string(addr.Country())
			_ = addr.ZipCode()
		}
		item := &onlineboutique_flat.CartItem{}
		for j := 0; j < obj.ItemsLength(); j++ {
			if obj.Items(item, j) {
				_ = string(item.ProductId())
				_ = item.Quantity()
			}
		}
	case "ShipOrderResponse":
		obj := onlineboutique_flat.GetRootAsShipOrderResponse(data, 0)
		_ = string(obj.TrackingId())
	default:
		panic(fmt.Sprintf("unsupported message type for FlatBuffers: %s", typeName))
	}
	return nil
}

// unmarshalCapnpAndAccessFields unmarshals a Cap'n Proto buffer and accesses all fields
func unmarshalCapnpAndAccessFields(typeName string, data []byte) error {
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		panic(fmt.Sprintf("failed to unmarshal Cap'n Proto message: %v", err))
	}

	switch typeName {
	case "Product":
		obj, err := onlineboutique_capnp.ReadRootProduct(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read Product: %v", err))
		}
		_, _ = obj.Id()
		_, _ = obj.Name()
		_, _ = obj.Description()
		_, _ = obj.Picture()
		if obj.HasPriceUsd() {
			price, _ := obj.PriceUsd()
			_, _ = price.CurrencyCode()
			_ = price.Units()
			_ = price.Nanos()
		}
		categories, _ := obj.Categories()
		for i := 0; i < categories.Len(); i++ {
			_, _ = categories.At(i)
		}
	case "CartItem":
		obj, err := onlineboutique_capnp.ReadRootCartItem(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read CartItem: %v", err))
		}
		_, _ = obj.ProductId()
		_ = obj.Quantity()
	case "Money":
		obj, err := onlineboutique_capnp.ReadRootMoney(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read Money: %v", err))
		}
		_, _ = obj.CurrencyCode()
		_ = obj.Units()
		_ = obj.Nanos()
	case "Address":
		obj, err := onlineboutique_capnp.ReadRootAddress(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read Address: %v", err))
		}
		_, _ = obj.StreetAddress()
		_, _ = obj.City()
		_, _ = obj.State()
		_, _ = obj.Country()
		_ = obj.ZipCode()
	case "GetProductRequest":
		obj, err := onlineboutique_capnp.ReadRootGetProductRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read GetProductRequest: %v", err))
		}
		_, _ = obj.Id()
	case "Empty":
		_, err := onlineboutique_capnp.ReadRootEmpty(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read Empty: %v", err))
		}
	case "EmptyUser":
		obj, err := onlineboutique_capnp.ReadRootEmptyUser(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read EmptyUser: %v", err))
		}
		_, _ = obj.UserId()
	case "Cart":
		obj, err := onlineboutique_capnp.ReadRootCart(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read Cart: %v", err))
		}
		_, _ = obj.UserId()
		items, _ := obj.Items()
		for i := 0; i < items.Len(); i++ {
			item := items.At(i)
			_, _ = item.ProductId()
			_ = item.Quantity()
		}
	case "AddItemRequest":
		obj, err := onlineboutique_capnp.ReadRootAddItemRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read AddItemRequest: %v", err))
		}
		_, _ = obj.UserId()
		if obj.HasItem() {
			item, _ := obj.Item()
			_, _ = item.ProductId()
			_ = item.Quantity()
		}
	case "GetQuoteRequest":
		obj, err := onlineboutique_capnp.ReadRootGetQuoteRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read GetQuoteRequest: %v", err))
		}
		if obj.HasAddress() {
			addr, _ := obj.Address()
			_, _ = addr.StreetAddress()
			_, _ = addr.City()
			_, _ = addr.State()
			_, _ = addr.Country()
			_ = addr.ZipCode()
		}
		items, _ := obj.Items()
		for i := 0; i < items.Len(); i++ {
			item := items.At(i)
			_, _ = item.ProductId()
			_ = item.Quantity()
		}
	case "GetQuoteResponse":
		obj, err := onlineboutique_capnp.ReadRootGetQuoteResponse(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read GetQuoteResponse: %v", err))
		}
		if obj.HasCostUsd() {
			cost, _ := obj.CostUsd()
			_, _ = cost.CurrencyCode()
			_ = cost.Units()
			_ = cost.Nanos()
		}
	case "ListProductsResponse":
		obj, err := onlineboutique_capnp.ReadRootListProductsResponse(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read ListProductsResponse: %v", err))
		}
		products, _ := obj.Products()
		for i := 0; i < products.Len(); i++ {
			product := products.At(i)
			_, _ = product.Id()
			_, _ = product.Name()
			_, _ = product.Description()
			_, _ = product.Picture()
			if product.HasPriceUsd() {
				price, _ := product.PriceUsd()
				_, _ = price.CurrencyCode()
				_ = price.Units()
				_ = price.Nanos()
			}
			categories, _ := product.Categories()
			for j := 0; j < categories.Len(); j++ {
				_, _ = categories.At(j)
			}
		}
	case "SearchProductsResponse":
		obj, err := onlineboutique_capnp.ReadRootSearchProductsResponse(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read SearchProductsResponse: %v", err))
		}
		results, _ := obj.Results()
		for i := 0; i < results.Len(); i++ {
			product := results.At(i)
			_, _ = product.Id()
			_, _ = product.Name()
			_, _ = product.Description()
			_, _ = product.Picture()
			if product.HasPriceUsd() {
				price, _ := product.PriceUsd()
				_, _ = price.CurrencyCode()
				_ = price.Units()
				_ = price.Nanos()
			}
			categories, _ := product.Categories()
			for j := 0; j < categories.Len(); j++ {
				_, _ = categories.At(j)
			}
		}
	case "Ad":
		obj, err := onlineboutique_capnp.ReadRootAd(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read Ad: %v", err))
		}
		_, _ = obj.RedirectUrl()
		_, _ = obj.Text()
	case "AdRequest":
		obj, err := onlineboutique_capnp.ReadRootAdRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read AdRequest: %v", err))
		}
		_, _ = obj.UserId()
		contextKeys, _ := obj.ContextKeys()
		for i := 0; i < contextKeys.Len(); i++ {
			_, _ = contextKeys.At(i)
		}
	case "AdResponse":
		obj, err := onlineboutique_capnp.ReadRootAdResponse(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read AdResponse: %v", err))
		}
		ads, _ := obj.Ads()
		for i := 0; i < ads.Len(); i++ {
			ad := ads.At(i)
			_, _ = ad.RedirectUrl()
			_, _ = ad.Text()
		}
	case "ChargeRequest":
		obj, err := onlineboutique_capnp.ReadRootChargeRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read ChargeRequest: %v", err))
		}
		if obj.HasAmount() {
			amount, _ := obj.Amount()
			_, _ = amount.CurrencyCode()
			_ = amount.Units()
			_ = amount.Nanos()
		}
		if obj.HasCreditCard() {
			card, _ := obj.CreditCard()
			_, _ = card.CreditCardNumber()
			_ = card.CreditCardCvv()
			_ = card.CreditCardExpirationYear()
			_ = card.CreditCardExpirationMonth()
		}
	case "ChargeResponse":
		obj, err := onlineboutique_capnp.ReadRootChargeResponse(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read ChargeResponse: %v", err))
		}
		_, _ = obj.TransactionId()
	case "CreditCardInfo":
		obj, err := onlineboutique_capnp.ReadRootCreditCardInfo(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read CreditCardInfo: %v", err))
		}
		_, _ = obj.CreditCardNumber()
		_ = obj.CreditCardCvv()
		_ = obj.CreditCardExpirationYear()
		_ = obj.CreditCardExpirationMonth()
	case "CurrencyConversionRequest":
		obj, err := onlineboutique_capnp.ReadRootCurrencyConversionRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read CurrencyConversionRequest: %v", err))
		}
		if obj.HasFrom() {
			from, _ := obj.From()
			_, _ = from.CurrencyCode()
			_ = from.Units()
			_ = from.Nanos()
		}
		_, _ = obj.ToCode()
		_, _ = obj.UserId()
	case "GetSupportedCurrenciesResponse":
		obj, err := onlineboutique_capnp.ReadRootGetSupportedCurrenciesResponse(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read GetSupportedCurrenciesResponse: %v", err))
		}
		codes, _ := obj.CurrencyCodes()
		for i := 0; i < codes.Len(); i++ {
			_, _ = codes.At(i)
		}
	case "ListRecommendationsRequest":
		obj, err := onlineboutique_capnp.ReadRootListRecommendationsRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read ListRecommendationsRequest: %v", err))
		}
		_, _ = obj.UserId()
		productIds, _ := obj.ProductIds()
		for i := 0; i < productIds.Len(); i++ {
			_, _ = productIds.At(i)
		}
	case "ListRecommendationsResponse":
		obj, err := onlineboutique_capnp.ReadRootListRecommendationsResponse(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read ListRecommendationsResponse: %v", err))
		}
		productIds, _ := obj.ProductIds()
		for i := 0; i < productIds.Len(); i++ {
			_, _ = productIds.At(i)
		}
	case "OrderItem":
		obj, err := onlineboutique_capnp.ReadRootOrderItem(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read OrderItem: %v", err))
		}
		if obj.HasItem() {
			item, _ := obj.Item()
			_, _ = item.ProductId()
			_ = item.Quantity()
		}
		if obj.HasCost() {
			cost, _ := obj.Cost()
			_, _ = cost.CurrencyCode()
			_ = cost.Units()
			_ = cost.Nanos()
		}
	case "OrderResult":
		obj, err := onlineboutique_capnp.ReadRootOrderResult(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read OrderResult: %v", err))
		}
		_, _ = obj.OrderId()
		_, _ = obj.ShippingTrackingId()
		if obj.HasShippingCost() {
			cost, _ := obj.ShippingCost()
			_, _ = cost.CurrencyCode()
			_ = cost.Units()
			_ = cost.Nanos()
		}
		if obj.HasShippingAddress() {
			addr, _ := obj.ShippingAddress()
			_, _ = addr.StreetAddress()
			_, _ = addr.City()
			_, _ = addr.State()
			_, _ = addr.Country()
			_ = addr.ZipCode()
		}
		items, _ := obj.Items()
		for i := 0; i < items.Len(); i++ {
			item := items.At(i)
			if item.HasItem() {
				itemItem, _ := item.Item()
				_, _ = itemItem.ProductId()
				_ = itemItem.Quantity()
			}
			if item.HasCost() {
				cost, _ := item.Cost()
				_, _ = cost.CurrencyCode()
				_ = cost.Units()
				_ = cost.Nanos()
			}
		}
	case "PlaceOrderRequest":
		obj, err := onlineboutique_capnp.ReadRootPlaceOrderRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read PlaceOrderRequest: %v", err))
		}
		_, _ = obj.UserId()
		_, _ = obj.UserCurrency()
		if obj.HasAddress() {
			addr, _ := obj.Address()
			_, _ = addr.StreetAddress()
			_, _ = addr.City()
			_, _ = addr.State()
			_, _ = addr.Country()
			_ = addr.ZipCode()
		}
		_, _ = obj.Email()
		if obj.HasCreditCard() {
			card, _ := obj.CreditCard()
			_, _ = card.CreditCardNumber()
			_ = card.CreditCardCvv()
			_ = card.CreditCardExpirationYear()
			_ = card.CreditCardExpirationMonth()
		}
	case "PlaceOrderResponse":
		obj, err := onlineboutique_capnp.ReadRootPlaceOrderResponse(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read PlaceOrderResponse: %v", err))
		}
		if obj.HasOrder() {
			order, _ := obj.Order()
			_, _ = order.OrderId()
			_, _ = order.ShippingTrackingId()
			if order.HasShippingCost() {
				cost, _ := order.ShippingCost()
				_, _ = cost.CurrencyCode()
				_ = cost.Units()
				_ = cost.Nanos()
			}
			if order.HasShippingAddress() {
				addr, _ := order.ShippingAddress()
				_, _ = addr.StreetAddress()
				_, _ = addr.City()
				_, _ = addr.State()
				_, _ = addr.Country()
				_ = addr.ZipCode()
			}
			items, _ := order.Items()
			for i := 0; i < items.Len(); i++ {
				item := items.At(i)
				if item.HasItem() {
					itemItem, _ := item.Item()
					_, _ = itemItem.ProductId()
					_ = itemItem.Quantity()
				}
				if item.HasCost() {
					cost, _ := item.Cost()
					_, _ = cost.CurrencyCode()
					_ = cost.Units()
					_ = cost.Nanos()
				}
			}
		}
	case "SendOrderConfirmationRequest":
		obj, err := onlineboutique_capnp.ReadRootSendOrderConfirmationRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read SendOrderConfirmationRequest: %v", err))
		}
		_, _ = obj.Email()
		if obj.HasOrder() {
			order, _ := obj.Order()
			_, _ = order.OrderId()
			_, _ = order.ShippingTrackingId()
			if order.HasShippingCost() {
				cost, _ := order.ShippingCost()
				_, _ = cost.CurrencyCode()
				_ = cost.Units()
				_ = cost.Nanos()
			}
			if order.HasShippingAddress() {
				addr, _ := order.ShippingAddress()
				_, _ = addr.StreetAddress()
				_, _ = addr.City()
				_, _ = addr.State()
				_, _ = addr.Country()
				_ = addr.ZipCode()
			}
			items, _ := order.Items()
			for i := 0; i < items.Len(); i++ {
				item := items.At(i)
				if item.HasItem() {
					itemItem, _ := item.Item()
					_, _ = itemItem.ProductId()
					_ = itemItem.Quantity()
				}
				if item.HasCost() {
					cost, _ := item.Cost()
					_, _ = cost.CurrencyCode()
					_ = cost.Units()
					_ = cost.Nanos()
				}
			}
		}
	case "ShipOrderRequest":
		obj, err := onlineboutique_capnp.ReadRootShipOrderRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read ShipOrderRequest: %v", err))
		}
		if obj.HasAddress() {
			addr, _ := obj.Address()
			_, _ = addr.StreetAddress()
			_, _ = addr.City()
			_, _ = addr.State()
			_, _ = addr.Country()
			_ = addr.ZipCode()
		}
		items, _ := obj.Items()
		for i := 0; i < items.Len(); i++ {
			item := items.At(i)
			_, _ = item.ProductId()
			_ = item.Quantity()
		}
	case "ShipOrderResponse":
		obj, err := onlineboutique_capnp.ReadRootShipOrderResponse(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read ShipOrderResponse: %v", err))
		}
		_, _ = obj.TrackingId()
	case "GetCartRequest":
		obj, err := onlineboutique_capnp.ReadRootGetCartRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read GetCartRequest: %v", err))
		}
		_, _ = obj.UserId()
	case "EmptyCartRequest":
		obj, err := onlineboutique_capnp.ReadRootEmptyCartRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read EmptyCartRequest: %v", err))
		}
		_, _ = obj.UserId()
	case "SearchProductsRequest":
		obj, err := onlineboutique_capnp.ReadRootSearchProductsRequest(msg)
		if err != nil {
			panic(fmt.Sprintf("failed to read SearchProductsRequest: %v", err))
		}
		_, _ = obj.Query()
	default:
		panic(fmt.Sprintf("unsupported message type for Cap'n Proto: %s", typeName))
	}
	return nil
}
