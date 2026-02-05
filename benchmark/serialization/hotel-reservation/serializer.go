package main

import (
	"fmt"

	"capnproto.org/go/capnp/v3"
	hotel_reservation_capnp "github.com/appnet-org/arpc/benchmark/serialization/hotel-reservation/capnp"
	hotel_reservation_flat "github.com/appnet-org/arpc/benchmark/serialization/hotel-reservation/flatbuffers/hotel_reservation"
	hotel_reservation "github.com/appnet-org/arpc/benchmark/serialization/hotel-reservation/proto"
	flatbuffers "github.com/google/flatbuffers/go"
	"google.golang.org/protobuf/proto"
)

// serializeProto serializes a proto message to protobuf format
func serializeProto(msg proto.Message) ([]byte, error) {
	return proto.Marshal(msg)
}

// serializeSymphony serializes a proto message to Symphony format
func serializeSymphony(msg proto.Message) ([]byte, error) {
	switch m := msg.(type) {
	case *hotel_reservation.NearbyRequest:
		return m.MarshalSymphony()
	case *hotel_reservation.NearbyResult:
		return m.MarshalSymphony()
	case *hotel_reservation.GetProfilesRequest:
		return m.MarshalSymphony()
	case *hotel_reservation.GetProfilesResult:
		return m.MarshalSymphony()
	case *hotel_reservation.Hotel:
		return m.MarshalSymphony()
	case *hotel_reservation.Address:
		return m.MarshalSymphony()
	case *hotel_reservation.Image:
		return m.MarshalSymphony()
	case *hotel_reservation.GetRecommendationsRequest:
		return m.MarshalSymphony()
	case *hotel_reservation.GetRecommendationsResult:
		return m.MarshalSymphony()
	case *hotel_reservation.GetRatesRequest:
		return m.MarshalSymphony()
	case *hotel_reservation.GetRatesResult:
		return m.MarshalSymphony()
	case *hotel_reservation.RatePlan:
		return m.MarshalSymphony()
	case *hotel_reservation.RoomType:
		return m.MarshalSymphony()
	case *hotel_reservation.ReservationRequest:
		return m.MarshalSymphony()
	case *hotel_reservation.ReservationResult:
		return m.MarshalSymphony()
	case *hotel_reservation.SearchRequest:
		return m.MarshalSymphony()
	case *hotel_reservation.SearchResult:
		return m.MarshalSymphony()
	case *hotel_reservation.CheckUserRequest:
		return m.MarshalSymphony()
	case *hotel_reservation.CheckUserResult:
		return m.MarshalSymphony()
	default:
		panic(fmt.Sprintf("unsupported message type for Symphony: %T", msg))
	}
}

// serializeSymphonyHybrid serializes a proto message to Symphony Hybrid format
func serializeSymphonyHybrid(msg proto.Message) ([]byte, error) {
	switch m := msg.(type) {
	case *hotel_reservation.NearbyRequest:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.NearbyResult:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.GetProfilesRequest:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.GetProfilesResult:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.Hotel:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.Address:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.Image:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.GetRecommendationsRequest:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.GetRecommendationsResult:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.GetRatesRequest:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.GetRatesResult:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.RatePlan:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.RoomType:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.ReservationRequest:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.ReservationResult:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.SearchRequest:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.SearchResult:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.CheckUserRequest:
		return m.MarshalSymphonyHybrid()
	case *hotel_reservation.CheckUserResult:
		return m.MarshalSymphonyHybrid()
	default:
		panic(fmt.Sprintf("unsupported message type for Symphony Hybrid: %T", msg))
	}
}

// unmarshalSymphony unmarshals Symphony format data into a proto message
func unmarshalSymphony(msg proto.Message, data []byte) error {
	switch m := msg.(type) {
	case *hotel_reservation.NearbyRequest:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.NearbyResult:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.GetProfilesRequest:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.GetProfilesResult:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.Hotel:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.Address:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.Image:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.GetRecommendationsRequest:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.GetRecommendationsResult:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.GetRatesRequest:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.GetRatesResult:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.RatePlan:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.RoomType:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.ReservationRequest:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.ReservationResult:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.SearchRequest:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.SearchResult:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.CheckUserRequest:
		return m.UnmarshalSymphony(data)
	case *hotel_reservation.CheckUserResult:
		return m.UnmarshalSymphony(data)
	default:
		panic(fmt.Sprintf("unsupported message type for Symphony: %T", msg))
	}
}

// unmarshalSymphonyHybrid unmarshals Symphony Hybrid format data into a proto message
func unmarshalSymphonyHybrid(msg proto.Message, data []byte) error {
	switch m := msg.(type) {
	case *hotel_reservation.NearbyRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.NearbyResult:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.GetProfilesRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.GetProfilesResult:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.Hotel:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.Address:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.Image:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.GetRecommendationsRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.GetRecommendationsResult:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.GetRatesRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.GetRatesResult:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.RatePlan:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.RoomType:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.ReservationRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.ReservationResult:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.SearchRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.SearchResult:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.CheckUserRequest:
		return m.UnmarshalSymphonyHybrid(data)
	case *hotel_reservation.CheckUserResult:
		return m.UnmarshalSymphonyHybrid(data)
	default:
		panic(fmt.Sprintf("unsupported message type for Symphony Hybrid: %T", msg))
	}
}

// accessAllFields accesses all fields of a proto message to ensure deserialization
func accessAllFields(msg proto.Message) {
	switch m := msg.(type) {
	case *hotel_reservation.NearbyRequest:
		_, _, _ = m.GetLat(), m.GetLon(), m.GetLatstring()
	case *hotel_reservation.NearbyResult:
		_ = m.GetHotelIds()
	case *hotel_reservation.GetProfilesRequest:
		_, _ = m.GetHotelIds(), m.GetLocale()
	case *hotel_reservation.GetProfilesResult:
		for _, h := range m.GetHotels() {
			accessAllFields(h)
		}
	case *hotel_reservation.Hotel:
		_, _, _, _ = m.GetId(), m.GetName(), m.GetPhoneNumber(), m.GetDescription()
		if addr := m.GetAddress(); addr != nil {
			accessAllFields(addr)
		}
		for _, img := range m.GetImages() {
			accessAllFields(img)
		}
	case *hotel_reservation.Address:
		_, _, _, _, _, _, _, _ = m.GetStreetNumber(), m.GetStreetName(), m.GetCity(), m.GetState(), m.GetCountry(), m.GetPostalCode(), m.GetLat(), m.GetLon()
	case *hotel_reservation.Image:
		_, _ = m.GetUrl(), m.GetDefault()
	case *hotel_reservation.GetRecommendationsRequest:
		_, _, _ = m.GetRequire(), m.GetLat(), m.GetLon()
	case *hotel_reservation.GetRecommendationsResult:
		_ = m.GetHotelIds()
	case *hotel_reservation.GetRatesRequest:
		_, _, _ = m.GetHotelIds(), m.GetInDate(), m.GetOutDate()
	case *hotel_reservation.GetRatesResult:
		for _, rp := range m.GetRatePlans() {
			accessAllFields(rp)
		}
	case *hotel_reservation.RatePlan:
		_, _, _, _ = m.GetHotelId(), m.GetCode(), m.GetInDate(), m.GetOutDate()
		if rt := m.GetRoomType(); rt != nil {
			accessAllFields(rt)
		}
	case *hotel_reservation.RoomType:
		_, _, _, _, _, _ = m.GetBookableRate(), m.GetTotalRate(), m.GetTotalRateInclusive(), m.GetCode(), m.GetCurrency(), m.GetRoomDescription()
	case *hotel_reservation.ReservationRequest:
		_, _, _, _, _ = m.GetCustomerName(), m.GetHotelId(), m.GetInDate(), m.GetOutDate(), m.GetRoomNumber()
	case *hotel_reservation.ReservationResult:
		_ = m.GetHotelId()
	case *hotel_reservation.SearchRequest:
		_, _, _, _ = m.GetLat(), m.GetLon(), m.GetInDate(), m.GetOutDate()
	case *hotel_reservation.SearchResult:
		_ = m.GetHotelIds()
	case *hotel_reservation.CheckUserRequest:
		_, _ = m.GetUsername(), m.GetPassword()
	case *hotel_reservation.CheckUserResult:
		_ = m.GetCorrect()
	default:
		panic(fmt.Errorf("unsupported message type for accessAllFields: %T", m))
	}
}

// serializeFlatbuffers serializes a proto message to FlatBuffers format
func serializeFlatbuffers(msg proto.Message) ([]byte, error) {
	builder := flatbuffers.NewBuilder(0)
	var offset flatbuffers.UOffsetT
	var err error

	switch m := msg.(type) {
	case *hotel_reservation.NearbyRequest:
		latstring := builder.CreateString(m.Latstring)
		hotel_reservation_flat.NearbyRequestStart(builder)
		hotel_reservation_flat.NearbyRequestAddLat(builder, m.Lat)
		hotel_reservation_flat.NearbyRequestAddLon(builder, m.Lon)
		hotel_reservation_flat.NearbyRequestAddLatstring(builder, latstring)
		offset = hotel_reservation_flat.NearbyRequestEnd(builder)
	case *hotel_reservation.NearbyResult:
		var hotelIdsOffset flatbuffers.UOffsetT
		if len(m.HotelIds) > 0 {
			ids := make([]flatbuffers.UOffsetT, len(m.HotelIds))
			for i, id := range m.HotelIds {
				ids[i] = builder.CreateString(id)
			}
			hotel_reservation_flat.NearbyResultStartHotelIdsVector(builder, len(m.HotelIds))
			for i := len(m.HotelIds) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(ids[i])
			}
			hotelIdsOffset = builder.EndVector(len(m.HotelIds))
		}
		hotel_reservation_flat.NearbyResultStart(builder)
		if len(m.HotelIds) > 0 {
			hotel_reservation_flat.NearbyResultAddHotelIds(builder, hotelIdsOffset)
		}
		offset = hotel_reservation_flat.NearbyResultEnd(builder)
	case *hotel_reservation.GetProfilesRequest:
		offset, err = serializeGetProfilesRequestFlatbuffers(builder, m)
	case *hotel_reservation.GetProfilesResult:
		offset, err = serializeGetProfilesResultFlatbuffers(builder, m)
	case *hotel_reservation.Hotel:
		offset, err = serializeHotelFlatbuffers(builder, m)
	case *hotel_reservation.Address:
		offset, err = serializeAddressFlatbuffers(builder, m)
	case *hotel_reservation.Image:
		url := builder.CreateString(m.Url)
		hotel_reservation_flat.ImageStart(builder)
		hotel_reservation_flat.ImageAddUrl(builder, url)
		hotel_reservation_flat.ImageAddDefault(builder, m.Default)
		offset = hotel_reservation_flat.ImageEnd(builder)
	case *hotel_reservation.GetRecommendationsRequest:
		require := builder.CreateString(m.Require)
		hotel_reservation_flat.GetRecommendationsRequestStart(builder)
		hotel_reservation_flat.GetRecommendationsRequestAddRequire(builder, require)
		hotel_reservation_flat.GetRecommendationsRequestAddLat(builder, m.Lat)
		hotel_reservation_flat.GetRecommendationsRequestAddLon(builder, m.Lon)
		offset = hotel_reservation_flat.GetRecommendationsRequestEnd(builder)
	case *hotel_reservation.GetRecommendationsResult:
		var hotelIdsOffset flatbuffers.UOffsetT
		if len(m.HotelIds) > 0 {
			ids := make([]flatbuffers.UOffsetT, len(m.HotelIds))
			for i, id := range m.HotelIds {
				ids[i] = builder.CreateString(id)
			}
			hotel_reservation_flat.GetRecommendationsResultStartHotelIdsVector(builder, len(m.HotelIds))
			for i := len(m.HotelIds) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(ids[i])
			}
			hotelIdsOffset = builder.EndVector(len(m.HotelIds))
		}
		hotel_reservation_flat.GetRecommendationsResultStart(builder)
		if len(m.HotelIds) > 0 {
			hotel_reservation_flat.GetRecommendationsResultAddHotelIds(builder, hotelIdsOffset)
		}
		offset = hotel_reservation_flat.GetRecommendationsResultEnd(builder)
	case *hotel_reservation.GetRatesRequest:
		offset, err = serializeGetRatesRequestFlatbuffers(builder, m)
	case *hotel_reservation.GetRatesResult:
		offset, err = serializeGetRatesResultFlatbuffers(builder, m)
	case *hotel_reservation.RatePlan:
		offset, err = serializeRatePlanFlatbuffers(builder, m)
	case *hotel_reservation.RoomType:
		offset, err = serializeRoomTypeFlatbuffers(builder, m)
	case *hotel_reservation.ReservationRequest:
		offset, err = serializeReservationRequestFlatbuffers(builder, m)
	case *hotel_reservation.ReservationResult:
		var hotelIdOffset flatbuffers.UOffsetT
		if len(m.HotelId) > 0 {
			ids := make([]flatbuffers.UOffsetT, len(m.HotelId))
			for i, id := range m.HotelId {
				ids[i] = builder.CreateString(id)
			}
			hotel_reservation_flat.ReservationResultStartHotelIdVector(builder, len(m.HotelId))
			for i := len(m.HotelId) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(ids[i])
			}
			hotelIdOffset = builder.EndVector(len(m.HotelId))
		}
		hotel_reservation_flat.ReservationResultStart(builder)
		if len(m.HotelId) > 0 {
			hotel_reservation_flat.ReservationResultAddHotelId(builder, hotelIdOffset)
		}
		offset = hotel_reservation_flat.ReservationResultEnd(builder)
	case *hotel_reservation.SearchRequest:
		inDate := builder.CreateString(m.InDate)
		outDate := builder.CreateString(m.OutDate)
		hotel_reservation_flat.SearchRequestStart(builder)
		hotel_reservation_flat.SearchRequestAddLat(builder, m.Lat)
		hotel_reservation_flat.SearchRequestAddLon(builder, m.Lon)
		hotel_reservation_flat.SearchRequestAddInDate(builder, inDate)
		hotel_reservation_flat.SearchRequestAddOutDate(builder, outDate)
		offset = hotel_reservation_flat.SearchRequestEnd(builder)
	case *hotel_reservation.SearchResult:
		var hotelIdsOffset flatbuffers.UOffsetT
		if len(m.HotelIds) > 0 {
			ids := make([]flatbuffers.UOffsetT, len(m.HotelIds))
			for i, id := range m.HotelIds {
				ids[i] = builder.CreateString(id)
			}
			hotel_reservation_flat.SearchResultStartHotelIdsVector(builder, len(m.HotelIds))
			for i := len(m.HotelIds) - 1; i >= 0; i-- {
				builder.PrependUOffsetT(ids[i])
			}
			hotelIdsOffset = builder.EndVector(len(m.HotelIds))
		}
		hotel_reservation_flat.SearchResultStart(builder)
		if len(m.HotelIds) > 0 {
			hotel_reservation_flat.SearchResultAddHotelIds(builder, hotelIdsOffset)
		}
		offset = hotel_reservation_flat.SearchResultEnd(builder)
	case *hotel_reservation.CheckUserRequest:
		username := builder.CreateString(m.Username)
		password := builder.CreateString(m.Password)
		hotel_reservation_flat.CheckUserRequestStart(builder)
		hotel_reservation_flat.CheckUserRequestAddUsername(builder, username)
		hotel_reservation_flat.CheckUserRequestAddPassword(builder, password)
		offset = hotel_reservation_flat.CheckUserRequestEnd(builder)
	case *hotel_reservation.CheckUserResult:
		hotel_reservation_flat.CheckUserResultStart(builder)
		hotel_reservation_flat.CheckUserResultAddCorrect(builder, m.Correct)
		offset = hotel_reservation_flat.CheckUserResultEnd(builder)
	default:
		panic(fmt.Sprintf("unsupported message type for FlatBuffers: %T", msg))
	}

	if err != nil {
		return nil, err
	}
	builder.Finish(offset)
	return builder.FinishedBytes(), nil
}

func serializeAddressFlatbuffers(builder *flatbuffers.Builder, a *hotel_reservation.Address) (flatbuffers.UOffsetT, error) {
	sn := builder.CreateString(a.StreetNumber)
	sname := builder.CreateString(a.StreetName)
	city := builder.CreateString(a.City)
	state := builder.CreateString(a.State)
	country := builder.CreateString(a.Country)
	pc := builder.CreateString(a.PostalCode)
	hotel_reservation_flat.AddressStart(builder)
	hotel_reservation_flat.AddressAddStreetNumber(builder, sn)
	hotel_reservation_flat.AddressAddStreetName(builder, sname)
	hotel_reservation_flat.AddressAddCity(builder, city)
	hotel_reservation_flat.AddressAddState(builder, state)
	hotel_reservation_flat.AddressAddCountry(builder, country)
	hotel_reservation_flat.AddressAddPostalCode(builder, pc)
	hotel_reservation_flat.AddressAddLat(builder, a.Lat)
	hotel_reservation_flat.AddressAddLon(builder, a.Lon)
	return hotel_reservation_flat.AddressEnd(builder), nil
}

func serializeImageFlatbuffers(builder *flatbuffers.Builder, img *hotel_reservation.Image) (flatbuffers.UOffsetT, error) {
	url := builder.CreateString(img.Url)
	hotel_reservation_flat.ImageStart(builder)
	hotel_reservation_flat.ImageAddUrl(builder, url)
	hotel_reservation_flat.ImageAddDefault(builder, img.Default)
	return hotel_reservation_flat.ImageEnd(builder), nil
}

func serializeHotelFlatbuffers(builder *flatbuffers.Builder, h *hotel_reservation.Hotel) (flatbuffers.UOffsetT, error) {
	id := builder.CreateString(h.Id)
	name := builder.CreateString(h.Name)
	phone := builder.CreateString(h.PhoneNumber)
	desc := builder.CreateString(h.Description)
	var addrOffset flatbuffers.UOffsetT
	if h.Address != nil {
		addrOffset, _ = serializeAddressFlatbuffers(builder, h.Address)
	}
	var imagesOffset flatbuffers.UOffsetT
	if len(h.Images) > 0 {
		imgOffsets := make([]flatbuffers.UOffsetT, len(h.Images))
		for i, img := range h.Images {
			imgOffsets[i], _ = serializeImageFlatbuffers(builder, img)
		}
		imagesOffset = hotel_reservation_flat.HotelStartImagesVector(builder, len(h.Images))
		for i := len(h.Images) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(imgOffsets[i])
		}
		imagesOffset = builder.EndVector(len(h.Images))
	}
	hotel_reservation_flat.HotelStart(builder)
	hotel_reservation_flat.HotelAddId(builder, id)
	hotel_reservation_flat.HotelAddName(builder, name)
	hotel_reservation_flat.HotelAddPhoneNumber(builder, phone)
	hotel_reservation_flat.HotelAddDescription(builder, desc)
	if h.Address != nil {
		hotel_reservation_flat.HotelAddAddress(builder, addrOffset)
	}
	if len(h.Images) > 0 {
		hotel_reservation_flat.HotelAddImages(builder, imagesOffset)
	}
	return hotel_reservation_flat.HotelEnd(builder), nil
}

func serializeGetProfilesRequestFlatbuffers(builder *flatbuffers.Builder, m *hotel_reservation.GetProfilesRequest) (flatbuffers.UOffsetT, error) {
	var hotelIdsOffset flatbuffers.UOffsetT
	if len(m.HotelIds) > 0 {
		ids := make([]flatbuffers.UOffsetT, len(m.HotelIds))
		for i, id := range m.HotelIds {
			ids[i] = builder.CreateString(id)
		}
		hotel_reservation_flat.GetProfilesRequestStartHotelIdsVector(builder, len(m.HotelIds))
		for i := len(m.HotelIds) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(ids[i])
		}
		hotelIdsOffset = builder.EndVector(len(m.HotelIds))
	}
	locale := builder.CreateString(m.Locale)
	hotel_reservation_flat.GetProfilesRequestStart(builder)
	if len(m.HotelIds) > 0 {
		hotel_reservation_flat.GetProfilesRequestAddHotelIds(builder, hotelIdsOffset)
	}
	hotel_reservation_flat.GetProfilesRequestAddLocale(builder, locale)
	return hotel_reservation_flat.GetProfilesRequestEnd(builder), nil
}

func serializeGetProfilesResultFlatbuffers(builder *flatbuffers.Builder, m *hotel_reservation.GetProfilesResult) (flatbuffers.UOffsetT, error) {
	var hotelsOffset flatbuffers.UOffsetT
	if len(m.Hotels) > 0 {
		hotelOffsets := make([]flatbuffers.UOffsetT, len(m.Hotels))
		for i, h := range m.Hotels {
			hotelOffsets[i], _ = serializeHotelFlatbuffers(builder, h)
		}
		hotelsOffset = hotel_reservation_flat.GetProfilesResultStartHotelsVector(builder, len(m.Hotels))
		for i := len(m.Hotels) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(hotelOffsets[i])
		}
		hotelsOffset = builder.EndVector(len(m.Hotels))
	}
	hotel_reservation_flat.GetProfilesResultStart(builder)
	if len(m.Hotels) > 0 {
		hotel_reservation_flat.GetProfilesResultAddHotels(builder, hotelsOffset)
	}
	return hotel_reservation_flat.GetProfilesResultEnd(builder), nil
}

func serializeRoomTypeFlatbuffers(builder *flatbuffers.Builder, m *hotel_reservation.RoomType) (flatbuffers.UOffsetT, error) {
	code := builder.CreateString(m.Code)
	currency := builder.CreateString(m.Currency)
	roomDesc := builder.CreateString(m.RoomDescription)
	hotel_reservation_flat.RoomTypeStart(builder)
	hotel_reservation_flat.RoomTypeAddBookableRate(builder, m.BookableRate)
	hotel_reservation_flat.RoomTypeAddTotalRate(builder, m.TotalRate)
	hotel_reservation_flat.RoomTypeAddTotalRateInclusive(builder, m.TotalRateInclusive)
	hotel_reservation_flat.RoomTypeAddCode(builder, code)
	hotel_reservation_flat.RoomTypeAddCurrency(builder, currency)
	hotel_reservation_flat.RoomTypeAddRoomDescription(builder, roomDesc)
	return hotel_reservation_flat.RoomTypeEnd(builder), nil
}

func serializeRatePlanFlatbuffers(builder *flatbuffers.Builder, m *hotel_reservation.RatePlan) (flatbuffers.UOffsetT, error) {
	hotelId := builder.CreateString(m.HotelId)
	code := builder.CreateString(m.Code)
	inDate := builder.CreateString(m.InDate)
	outDate := builder.CreateString(m.OutDate)
	var roomTypeOffset flatbuffers.UOffsetT
	if m.RoomType != nil {
		roomTypeOffset, _ = serializeRoomTypeFlatbuffers(builder, m.RoomType)
	}
	hotel_reservation_flat.RatePlanStart(builder)
	hotel_reservation_flat.RatePlanAddHotelId(builder, hotelId)
	hotel_reservation_flat.RatePlanAddCode(builder, code)
	hotel_reservation_flat.RatePlanAddInDate(builder, inDate)
	hotel_reservation_flat.RatePlanAddOutDate(builder, outDate)
	if m.RoomType != nil {
		hotel_reservation_flat.RatePlanAddRoomType(builder, roomTypeOffset)
	}
	return hotel_reservation_flat.RatePlanEnd(builder), nil
}

func serializeGetRatesRequestFlatbuffers(builder *flatbuffers.Builder, m *hotel_reservation.GetRatesRequest) (flatbuffers.UOffsetT, error) {
	var hotelIdsOffset flatbuffers.UOffsetT
	if len(m.HotelIds) > 0 {
		ids := make([]flatbuffers.UOffsetT, len(m.HotelIds))
		for i, id := range m.HotelIds {
			ids[i] = builder.CreateString(id)
		}
		hotel_reservation_flat.GetRatesRequestStartHotelIdsVector(builder, len(m.HotelIds))
		for i := len(m.HotelIds) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(ids[i])
		}
		hotelIdsOffset = builder.EndVector(len(m.HotelIds))
	}
	inDate := builder.CreateString(m.InDate)
	outDate := builder.CreateString(m.OutDate)
	hotel_reservation_flat.GetRatesRequestStart(builder)
	if len(m.HotelIds) > 0 {
		hotel_reservation_flat.GetRatesRequestAddHotelIds(builder, hotelIdsOffset)
	}
	hotel_reservation_flat.GetRatesRequestAddInDate(builder, inDate)
	hotel_reservation_flat.GetRatesRequestAddOutDate(builder, outDate)
	return hotel_reservation_flat.GetRatesRequestEnd(builder), nil
}

func serializeGetRatesResultFlatbuffers(builder *flatbuffers.Builder, m *hotel_reservation.GetRatesResult) (flatbuffers.UOffsetT, error) {
	var ratePlansOffset flatbuffers.UOffsetT
	if len(m.RatePlans) > 0 {
		rpOffsets := make([]flatbuffers.UOffsetT, len(m.RatePlans))
		for i, rp := range m.RatePlans {
			rpOffsets[i], _ = serializeRatePlanFlatbuffers(builder, rp)
		}
		ratePlansOffset = hotel_reservation_flat.GetRatesResultStartRatePlansVector(builder, len(m.RatePlans))
		for i := len(m.RatePlans) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(rpOffsets[i])
		}
		ratePlansOffset = builder.EndVector(len(m.RatePlans))
	}
	hotel_reservation_flat.GetRatesResultStart(builder)
	if len(m.RatePlans) > 0 {
		hotel_reservation_flat.GetRatesResultAddRatePlans(builder, ratePlansOffset)
	}
	return hotel_reservation_flat.GetRatesResultEnd(builder), nil
}

func serializeReservationRequestFlatbuffers(builder *flatbuffers.Builder, m *hotel_reservation.ReservationRequest) (flatbuffers.UOffsetT, error) {
	customerName := builder.CreateString(m.CustomerName)
	var hotelIdOffset flatbuffers.UOffsetT
	if len(m.HotelId) > 0 {
		ids := make([]flatbuffers.UOffsetT, len(m.HotelId))
		for i, id := range m.HotelId {
			ids[i] = builder.CreateString(id)
		}
		hotel_reservation_flat.ReservationRequestStartHotelIdVector(builder, len(m.HotelId))
		for i := len(m.HotelId) - 1; i >= 0; i-- {
			builder.PrependUOffsetT(ids[i])
		}
		hotelIdOffset = builder.EndVector(len(m.HotelId))
	}
	inDate := builder.CreateString(m.InDate)
	outDate := builder.CreateString(m.OutDate)
	hotel_reservation_flat.ReservationRequestStart(builder)
	hotel_reservation_flat.ReservationRequestAddCustomerName(builder, customerName)
	if len(m.HotelId) > 0 {
		hotel_reservation_flat.ReservationRequestAddHotelId(builder, hotelIdOffset)
	}
	hotel_reservation_flat.ReservationRequestAddInDate(builder, inDate)
	hotel_reservation_flat.ReservationRequestAddOutDate(builder, outDate)
	hotel_reservation_flat.ReservationRequestAddRoomNumber(builder, m.RoomNumber)
	return hotel_reservation_flat.ReservationRequestEnd(builder), nil
}

// serializeCapnp serializes a proto message to Cap'n Proto format
func serializeCapnp(msg proto.Message) ([]byte, error) {
	msgCapnp, seg, err := capnp.NewMessage(capnp.SingleSegment(nil))
	if err != nil {
		return nil, fmt.Errorf("capnp NewMessage: %w", err)
	}

	switch m := msg.(type) {
	case *hotel_reservation.NearbyRequest:
		root, err := hotel_reservation_capnp.NewRootNearbyRequest(seg)
		if err != nil {
			return nil, err
		}
		root.SetLat(m.Lat)
		root.SetLon(m.Lon)
		if err := root.SetLatstring(m.Latstring); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.NearbyResult:
		root, err := hotel_reservation_capnp.NewRootNearbyResult(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeNearbyResultCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.GetProfilesRequest:
		root, err := hotel_reservation_capnp.NewRootGetProfilesRequest(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeGetProfilesRequestCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.GetProfilesResult:
		root, err := hotel_reservation_capnp.NewRootGetProfilesResult(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeGetProfilesResultCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.Hotel:
		root, err := hotel_reservation_capnp.NewRootHotel(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeHotelCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.Address:
		root, err := hotel_reservation_capnp.NewRootAddress(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeAddressCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.Image:
		root, err := hotel_reservation_capnp.NewRootImage(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeImageCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.GetRecommendationsRequest:
		root, err := hotel_reservation_capnp.NewRootGetRecommendationsRequest(seg)
		if err != nil {
			return nil, err
		}
		if err := root.SetRequire(m.Require); err != nil {
			return nil, err
		}
		root.SetLat(m.Lat)
		root.SetLon(m.Lon)
		return msgCapnp.Marshal()
	case *hotel_reservation.GetRecommendationsResult:
		root, err := hotel_reservation_capnp.NewRootGetRecommendationsResult(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeGetRecommendationsResultCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.GetRatesRequest:
		root, err := hotel_reservation_capnp.NewRootGetRatesRequest(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeGetRatesRequestCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.GetRatesResult:
		root, err := hotel_reservation_capnp.NewRootGetRatesResult(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeGetRatesResultCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.RatePlan:
		root, err := hotel_reservation_capnp.NewRootRatePlan(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeRatePlanCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.RoomType:
		root, err := hotel_reservation_capnp.NewRootRoomType(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeRoomTypeCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.ReservationRequest:
		root, err := hotel_reservation_capnp.NewRootReservationRequest(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeReservationRequestCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.ReservationResult:
		root, err := hotel_reservation_capnp.NewRootReservationResult(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeReservationResultCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.SearchRequest:
		root, err := hotel_reservation_capnp.NewRootSearchRequest(seg)
		if err != nil {
			return nil, err
		}
		root.SetLat(m.Lat)
		root.SetLon(m.Lon)
		if err := root.SetInDate(m.InDate); err != nil {
			return nil, err
		}
		if err := root.SetOutDate(m.OutDate); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.SearchResult:
		root, err := hotel_reservation_capnp.NewRootSearchResult(seg)
		if err != nil {
			return nil, err
		}
		if err := serializeSearchResultCapnp(root, m); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.CheckUserRequest:
		root, err := hotel_reservation_capnp.NewRootCheckUserRequest(seg)
		if err != nil {
			return nil, err
		}
		if err := root.SetUsername(m.Username); err != nil {
			return nil, err
		}
		if err := root.SetPassword(m.Password); err != nil {
			return nil, err
		}
		return msgCapnp.Marshal()
	case *hotel_reservation.CheckUserResult:
		root, err := hotel_reservation_capnp.NewRootCheckUserResult(seg)
		if err != nil {
			return nil, err
		}
		root.SetCorrect(m.Correct)
		return msgCapnp.Marshal()
	default:
		panic(fmt.Sprintf("unsupported message type for Cap'n Proto: %T", msg))
	}
}

func serializeAddressCapnp(s hotel_reservation_capnp.Address, a *hotel_reservation.Address) error {
	if err := s.SetStreetNumber(a.StreetNumber); err != nil {
		return err
	}
	if err := s.SetStreetName(a.StreetName); err != nil {
		return err
	}
	if err := s.SetCity(a.City); err != nil {
		return err
	}
	if err := s.SetState(a.State); err != nil {
		return err
	}
	if err := s.SetCountry(a.Country); err != nil {
		return err
	}
	if err := s.SetPostalCode(a.PostalCode); err != nil {
		return err
	}
	s.SetLat(a.Lat)
	s.SetLon(a.Lon)
	return nil
}

func serializeImageCapnp(s hotel_reservation_capnp.Image, img *hotel_reservation.Image) error {
	if err := s.SetUrl(img.Url); err != nil {
		return err
	}
	s.SetDefault(img.Default)
	return nil
}

func serializeHotelCapnp(s hotel_reservation_capnp.Hotel, h *hotel_reservation.Hotel) error {
	if err := s.SetId(h.Id); err != nil {
		return err
	}
	if err := s.SetName(h.Name); err != nil {
		return err
	}
	if err := s.SetPhoneNumber(h.PhoneNumber); err != nil {
		return err
	}
	if err := s.SetDescription(h.Description); err != nil {
		return err
	}
	if h.Address != nil {
		addr, err := s.NewAddress()
		if err != nil {
			return err
		}
		if err := serializeAddressCapnp(addr, h.Address); err != nil {
			return err
		}
	}
	if len(h.Images) > 0 {
		list, err := s.NewImages(int32(len(h.Images)))
		if err != nil {
			return err
		}
		for i := range h.Images {
			if err := serializeImageCapnp(list.At(i), h.Images[i]); err != nil {
				return err
			}
		}
	}
	return nil
}

func serializeNearbyResultCapnp(s hotel_reservation_capnp.NearbyResult, m *hotel_reservation.NearbyResult) error {
	if len(m.HotelIds) == 0 {
		return nil
	}
	list, err := s.NewHotelIds(int32(len(m.HotelIds)))
	if err != nil {
		return err
	}
	for i, id := range m.HotelIds {
		if err := list.Set(i, id); err != nil {
			return err
		}
	}
	return nil
}

func serializeGetProfilesRequestCapnp(s hotel_reservation_capnp.GetProfilesRequest, m *hotel_reservation.GetProfilesRequest) error {
	if len(m.HotelIds) > 0 {
		list, err := s.NewHotelIds(int32(len(m.HotelIds)))
		if err != nil {
			return err
		}
		for i, id := range m.HotelIds {
			if err := list.Set(i, id); err != nil {
				return err
			}
		}
	}
	return s.SetLocale(m.Locale)
}

func serializeGetProfilesResultCapnp(s hotel_reservation_capnp.GetProfilesResult, m *hotel_reservation.GetProfilesResult) error {
	if len(m.Hotels) == 0 {
		return nil
	}
	list, err := s.NewHotels(int32(len(m.Hotels)))
	if err != nil {
		return err
	}
	for i := range m.Hotels {
		if err := serializeHotelCapnp(list.At(i), m.Hotels[i]); err != nil {
			return err
		}
	}
	return nil
}

func serializeGetRecommendationsResultCapnp(s hotel_reservation_capnp.GetRecommendationsResult, m *hotel_reservation.GetRecommendationsResult) error {
	if len(m.HotelIds) == 0 {
		return nil
	}
	list, err := s.NewHotelIds(int32(len(m.HotelIds)))
	if err != nil {
		return err
	}
	for i, id := range m.HotelIds {
		if err := list.Set(i, id); err != nil {
			return err
		}
	}
	return nil
}

func serializeGetRatesRequestCapnp(s hotel_reservation_capnp.GetRatesRequest, m *hotel_reservation.GetRatesRequest) error {
	if len(m.HotelIds) > 0 {
		list, err := s.NewHotelIds(int32(len(m.HotelIds)))
		if err != nil {
			return err
		}
		for i, id := range m.HotelIds {
			if err := list.Set(i, id); err != nil {
				return err
			}
		}
	}
	if err := s.SetInDate(m.InDate); err != nil {
		return err
	}
	return s.SetOutDate(m.OutDate)
}

func serializeGetRatesResultCapnp(s hotel_reservation_capnp.GetRatesResult, m *hotel_reservation.GetRatesResult) error {
	if len(m.RatePlans) == 0 {
		return nil
	}
	list, err := s.NewRatePlans(int32(len(m.RatePlans)))
	if err != nil {
		return err
	}
	for i := range m.RatePlans {
		if err := serializeRatePlanCapnp(list.At(i), m.RatePlans[i]); err != nil {
			return err
		}
	}
	return nil
}

func serializeRoomTypeCapnp(s hotel_reservation_capnp.RoomType, m *hotel_reservation.RoomType) error {
	s.SetBookableRate(m.BookableRate)
	s.SetTotalRate(m.TotalRate)
	s.SetTotalRateInclusive(m.TotalRateInclusive)
	if err := s.SetCode(m.Code); err != nil {
		return err
	}
	if err := s.SetCurrency(m.Currency); err != nil {
		return err
	}
	return s.SetRoomDescription(m.RoomDescription)
}

func serializeRatePlanCapnp(s hotel_reservation_capnp.RatePlan, m *hotel_reservation.RatePlan) error {
	if err := s.SetHotelId(m.HotelId); err != nil {
		return err
	}
	if err := s.SetCode(m.Code); err != nil {
		return err
	}
	if err := s.SetInDate(m.InDate); err != nil {
		return err
	}
	if err := s.SetOutDate(m.OutDate); err != nil {
		return err
	}
	if m.RoomType != nil {
		rt, err := s.NewRoomType()
		if err != nil {
			return err
		}
		if err := serializeRoomTypeCapnp(rt, m.RoomType); err != nil {
			return err
		}
	}
	return nil
}

func serializeReservationRequestCapnp(s hotel_reservation_capnp.ReservationRequest, m *hotel_reservation.ReservationRequest) error {
	if err := s.SetCustomerName(m.CustomerName); err != nil {
		return err
	}
	if len(m.HotelId) > 0 {
		list, err := s.NewHotelId(int32(len(m.HotelId)))
		if err != nil {
			return err
		}
		for i, id := range m.HotelId {
			if err := list.Set(i, id); err != nil {
				return err
			}
		}
	}
	if err := s.SetInDate(m.InDate); err != nil {
		return err
	}
	if err := s.SetOutDate(m.OutDate); err != nil {
		return err
	}
	s.SetRoomNumber(m.RoomNumber)
	return nil
}

func serializeReservationResultCapnp(s hotel_reservation_capnp.ReservationResult, m *hotel_reservation.ReservationResult) error {
	if len(m.HotelId) == 0 {
		return nil
	}
	list, err := s.NewHotelId(int32(len(m.HotelId)))
	if err != nil {
		return err
	}
	for i, id := range m.HotelId {
		if err := list.Set(i, id); err != nil {
			return err
		}
	}
	return nil
}

func serializeSearchResultCapnp(s hotel_reservation_capnp.SearchResult, m *hotel_reservation.SearchResult) error {
	if len(m.HotelIds) == 0 {
		return nil
	}
	list, err := s.NewHotelIds(int32(len(m.HotelIds)))
	if err != nil {
		return err
	}
	for i, id := range m.HotelIds {
		if err := list.Set(i, id); err != nil {
			return err
		}
	}
	return nil
}

// unmarshalFlatbuffersAndAccessFields unmarshals a FlatBuffers buffer and accesses all fields
func unmarshalFlatbuffersAndAccessFields(typeName string, data []byte) error {
	switch typeName {
	case "NearbyRequest":
		obj := hotel_reservation_flat.GetRootAsNearbyRequest(data, 0)
		_, _, _ = obj.Lat(), obj.Lon(), string(obj.Latstring())
	case "NearbyResult":
		obj := hotel_reservation_flat.GetRootAsNearbyResult(data, 0)
		for j := 0; j < obj.HotelIdsLength(); j++ {
			_ = string(obj.HotelIds(j))
		}
	case "GetProfilesRequest":
		obj := hotel_reservation_flat.GetRootAsGetProfilesRequest(data, 0)
		for j := 0; j < obj.HotelIdsLength(); j++ {
			_ = string(obj.HotelIds(j))
		}
		_ = string(obj.Locale())
	case "GetProfilesResult":
		obj := hotel_reservation_flat.GetRootAsGetProfilesResult(data, 0)
		hotel := &hotel_reservation_flat.Hotel{}
		for j := 0; j < obj.HotelsLength(); j++ {
			if obj.Hotels(hotel, j) {
				_, _, _, _ = string(hotel.Id()), string(hotel.Name()), string(hotel.PhoneNumber()), string(hotel.Description())
				if addr := hotel.Address(nil); addr != nil {
					_, _, _, _, _, _, _, _ = string(addr.StreetNumber()), string(addr.StreetName()), string(addr.City()), string(addr.State()), string(addr.Country()), string(addr.PostalCode()), addr.Lat(), addr.Lon()
				}
				for k := 0; k < hotel.ImagesLength(); k++ {
					img := &hotel_reservation_flat.Image{}
					if hotel.Images(img, k) {
						_, _ = string(img.Url()), img.Default()
					}
				}
			}
		}
	case "Hotel":
		obj := hotel_reservation_flat.GetRootAsHotel(data, 0)
		_, _, _, _ = string(obj.Id()), string(obj.Name()), string(obj.PhoneNumber()), string(obj.Description())
		if addr := obj.Address(nil); addr != nil {
			_, _, _, _, _, _, _, _ = string(addr.StreetNumber()), string(addr.StreetName()), string(addr.City()), string(addr.State()), string(addr.Country()), string(addr.PostalCode()), addr.Lat(), addr.Lon()
		}
		img := &hotel_reservation_flat.Image{}
		for j := 0; j < obj.ImagesLength(); j++ {
			if obj.Images(img, j) {
				_, _ = string(img.Url()), img.Default()
			}
		}
	case "Address":
		obj := hotel_reservation_flat.GetRootAsAddress(data, 0)
		_, _, _, _, _, _, _, _ = string(obj.StreetNumber()), string(obj.StreetName()), string(obj.City()), string(obj.State()), string(obj.Country()), string(obj.PostalCode()), obj.Lat(), obj.Lon()
	case "Image":
		obj := hotel_reservation_flat.GetRootAsImage(data, 0)
		_, _ = string(obj.Url()), obj.Default()
	case "GetRecommendationsRequest":
		obj := hotel_reservation_flat.GetRootAsGetRecommendationsRequest(data, 0)
		_, _, _ = string(obj.Require()), obj.Lat(), obj.Lon()
	case "GetRecommendationsResult":
		obj := hotel_reservation_flat.GetRootAsGetRecommendationsResult(data, 0)
		for j := 0; j < obj.HotelIdsLength(); j++ {
			_ = string(obj.HotelIds(j))
		}
	case "GetRatesRequest":
		obj := hotel_reservation_flat.GetRootAsGetRatesRequest(data, 0)
		for j := 0; j < obj.HotelIdsLength(); j++ {
			_ = string(obj.HotelIds(j))
		}
		_, _ = string(obj.InDate()), string(obj.OutDate())
	case "GetRatesResult":
		obj := hotel_reservation_flat.GetRootAsGetRatesResult(data, 0)
		rp := &hotel_reservation_flat.RatePlan{}
		rt := &hotel_reservation_flat.RoomType{}
		for j := 0; j < obj.RatePlansLength(); j++ {
			if obj.RatePlans(rp, j) {
				_, _, _, _ = string(rp.HotelId()), string(rp.Code()), string(rp.InDate()), string(rp.OutDate())
				if rp.RoomType(rt) != nil {
					_, _, _, _, _, _ = rt.BookableRate(), rt.TotalRate(), rt.TotalRateInclusive(), string(rt.Code()), string(rt.Currency()), string(rt.RoomDescription())
				}
			}
		}
	case "RatePlan":
		obj := hotel_reservation_flat.GetRootAsRatePlan(data, 0)
		_, _, _, _ = string(obj.HotelId()), string(obj.Code()), string(obj.InDate()), string(obj.OutDate())
		if rt := obj.RoomType(nil); rt != nil {
			_, _, _, _, _, _ = rt.BookableRate(), rt.TotalRate(), rt.TotalRateInclusive(), string(rt.Code()), string(rt.Currency()), string(rt.RoomDescription())
		}
	case "RoomType":
		obj := hotel_reservation_flat.GetRootAsRoomType(data, 0)
		_, _, _, _, _, _ = obj.BookableRate(), obj.TotalRate(), obj.TotalRateInclusive(), string(obj.Code()), string(obj.Currency()), string(obj.RoomDescription())
	case "ReservationRequest":
		obj := hotel_reservation_flat.GetRootAsReservationRequest(data, 0)
		_ = string(obj.CustomerName())
		for j := 0; j < obj.HotelIdLength(); j++ {
			_ = string(obj.HotelId(j))
		}
		_, _, _ = string(obj.InDate()), string(obj.OutDate()), obj.RoomNumber()
	case "ReservationResult":
		obj := hotel_reservation_flat.GetRootAsReservationResult(data, 0)
		for j := 0; j < obj.HotelIdLength(); j++ {
			_ = string(obj.HotelId(j))
		}
	case "SearchRequest":
		obj := hotel_reservation_flat.GetRootAsSearchRequest(data, 0)
		_, _, _, _ = obj.Lat(), obj.Lon(), string(obj.InDate()), string(obj.OutDate())
	case "SearchResult":
		obj := hotel_reservation_flat.GetRootAsSearchResult(data, 0)
		for j := 0; j < obj.HotelIdsLength(); j++ {
			_ = string(obj.HotelIds(j))
		}
	case "CheckUserRequest":
		obj := hotel_reservation_flat.GetRootAsCheckUserRequest(data, 0)
		_, _ = string(obj.Username()), string(obj.Password())
	case "CheckUserResult":
		obj := hotel_reservation_flat.GetRootAsCheckUserResult(data, 0)
		_ = obj.Correct()
	default:
		panic(fmt.Sprintf("unsupported message type for FlatBuffers: %s", typeName))
	}
	return nil
}

// unmarshalCapnpAndAccessFields unmarshals a Cap'n Proto buffer and accesses all fields
func unmarshalCapnpAndAccessFields(typeName string, data []byte) error {
	msg, err := capnp.Unmarshal(data)
	if err != nil {
		return fmt.Errorf("capnp Unmarshal: %w", err)
	}

	switch typeName {
	case "NearbyRequest":
		obj, err := hotel_reservation_capnp.ReadRootNearbyRequest(msg)
		if err != nil {
			return err
		}
		_, _ = obj.Lat(), obj.Lon()
		_, _ = obj.Latstring()
	case "NearbyResult":
		obj, err := hotel_reservation_capnp.ReadRootNearbyResult(msg)
		if err != nil {
			return err
		}
		ids, _ := obj.HotelIds()
		for i := 0; i < ids.Len(); i++ {
			_, _ = ids.At(i)
		}
	case "GetProfilesRequest":
		obj, err := hotel_reservation_capnp.ReadRootGetProfilesRequest(msg)
		if err != nil {
			return err
		}
		ids, _ := obj.HotelIds()
		for i := 0; i < ids.Len(); i++ {
			_, _ = ids.At(i)
		}
		_, _ = obj.Locale()
	case "GetProfilesResult":
		obj, err := hotel_reservation_capnp.ReadRootGetProfilesResult(msg)
		if err != nil {
			return err
		}
		hotels, _ := obj.Hotels()
		for i := 0; i < hotels.Len(); i++ {
			h := hotels.At(i)
			_, _ = h.Id()
			_, _ = h.Name()
			_, _ = h.PhoneNumber()
			_, _ = h.Description()
			if h.HasAddress() {
				addr, _ := h.Address()
				_, _ = addr.StreetNumber()
				_, _ = addr.StreetName()
				_, _ = addr.City()
				_, _ = addr.State()
				_, _ = addr.Country()
				_, _ = addr.PostalCode()
				_, _ = addr.Lat(), addr.Lon()
			}
			imgs, _ := h.Images()
			for j := 0; j < imgs.Len(); j++ {
				img := imgs.At(j)
				_, _ = img.Url()
				_ = img.Default()
			}
		}
	case "Hotel":
		obj, err := hotel_reservation_capnp.ReadRootHotel(msg)
		if err != nil {
			return err
		}
		_, _ = obj.Id()
		_, _ = obj.Name()
		_, _ = obj.PhoneNumber()
		_, _ = obj.Description()
		if obj.HasAddress() {
			addr, _ := obj.Address()
			_, _ = addr.StreetNumber()
			_, _ = addr.StreetName()
			_, _ = addr.City()
			_, _ = addr.State()
			_, _ = addr.Country()
			_, _ = addr.PostalCode()
			_, _ = addr.Lat(), addr.Lon()
		}
		imgs, _ := obj.Images()
		for i := 0; i < imgs.Len(); i++ {
			img := imgs.At(i)
			_, _ = img.Url()
			_ = img.Default()
		}
	case "Address":
		obj, err := hotel_reservation_capnp.ReadRootAddress(msg)
		if err != nil {
			return err
		}
		_, _ = obj.StreetNumber()
		_, _ = obj.StreetName()
		_, _ = obj.City()
		_, _ = obj.State()
		_, _ = obj.Country()
		_, _ = obj.PostalCode()
		_, _ = obj.Lat(), obj.Lon()
	case "Image":
		obj, err := hotel_reservation_capnp.ReadRootImage(msg)
		if err != nil {
			return err
		}
		_, _ = obj.Url()
		_ = obj.Default()
	case "GetRecommendationsRequest":
		obj, err := hotel_reservation_capnp.ReadRootGetRecommendationsRequest(msg)
		if err != nil {
			return err
		}
		_, _ = obj.Require()
		_, _ = obj.Lat(), obj.Lon()
	case "GetRecommendationsResult":
		obj, err := hotel_reservation_capnp.ReadRootGetRecommendationsResult(msg)
		if err != nil {
			return err
		}
		ids, _ := obj.HotelIds()
		for i := 0; i < ids.Len(); i++ {
			_, _ = ids.At(i)
		}
	case "GetRatesRequest":
		obj, err := hotel_reservation_capnp.ReadRootGetRatesRequest(msg)
		if err != nil {
			return err
		}
		ids, _ := obj.HotelIds()
		for i := 0; i < ids.Len(); i++ {
			_, _ = ids.At(i)
		}
		_, _ = obj.InDate()
		_, _ = obj.OutDate()
	case "GetRatesResult":
		obj, err := hotel_reservation_capnp.ReadRootGetRatesResult(msg)
		if err != nil {
			return err
		}
		rps, _ := obj.RatePlans()
		for i := 0; i < rps.Len(); i++ {
			rp := rps.At(i)
			_, _ = rp.HotelId()
			_, _ = rp.Code()
			_, _ = rp.InDate()
			_, _ = rp.OutDate()
			if rp.HasRoomType() {
				rt, _ := rp.RoomType()
				_, _, _ = rt.BookableRate(), rt.TotalRate(), rt.TotalRateInclusive()
				_, _ = rt.Code()
				_, _ = rt.Currency()
				_, _ = rt.RoomDescription()
			}
		}
	case "RatePlan":
		obj, err := hotel_reservation_capnp.ReadRootRatePlan(msg)
		if err != nil {
			return err
		}
		_, _ = obj.HotelId()
		_, _ = obj.Code()
		_, _ = obj.InDate()
		_, _ = obj.OutDate()
		if obj.HasRoomType() {
			rt, _ := obj.RoomType()
			_, _, _ = rt.BookableRate(), rt.TotalRate(), rt.TotalRateInclusive()
			_, _ = rt.Code()
			_, _ = rt.Currency()
			_, _ = rt.RoomDescription()
		}
	case "RoomType":
		obj, err := hotel_reservation_capnp.ReadRootRoomType(msg)
		if err != nil {
			return err
		}
		_, _, _ = obj.BookableRate(), obj.TotalRate(), obj.TotalRateInclusive()
		_, _ = obj.Code()
		_, _ = obj.Currency()
		_, _ = obj.RoomDescription()
	case "ReservationRequest":
		obj, err := hotel_reservation_capnp.ReadRootReservationRequest(msg)
		if err != nil {
			return err
		}
		_, _ = obj.CustomerName()
		_ = obj.RoomNumber()
		ids, _ := obj.HotelId()
		for i := 0; i < ids.Len(); i++ {
			_, _ = ids.At(i)
		}
		_, _ = obj.InDate()
		_, _ = obj.OutDate()
	case "ReservationResult":
		obj, err := hotel_reservation_capnp.ReadRootReservationResult(msg)
		if err != nil {
			return err
		}
		ids, _ := obj.HotelId()
		for i := 0; i < ids.Len(); i++ {
			_, _ = ids.At(i)
		}
	case "SearchRequest":
		obj, err := hotel_reservation_capnp.ReadRootSearchRequest(msg)
		if err != nil {
			return err
		}
		_, _ = obj.Lat(), obj.Lon()
		_, _ = obj.InDate()
		_, _ = obj.OutDate()
	case "SearchResult":
		obj, err := hotel_reservation_capnp.ReadRootSearchResult(msg)
		if err != nil {
			return err
		}
		ids, _ := obj.HotelIds()
		for i := 0; i < ids.Len(); i++ {
			_, _ = ids.At(i)
		}
	case "CheckUserRequest":
		obj, err := hotel_reservation_capnp.ReadRootCheckUserRequest(msg)
		if err != nil {
			return err
		}
		_, _ = obj.Username()
		_, _ = obj.Password()
	case "CheckUserResult":
		obj, err := hotel_reservation_capnp.ReadRootCheckUserResult(msg)
		if err != nil {
			return err
		}
		_ = obj.Correct()
	default:
		panic(fmt.Sprintf("unsupported message type for Cap'n Proto: %s", typeName))
	}
	return nil
}
