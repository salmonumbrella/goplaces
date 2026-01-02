// Package goplaces provides a Go client for the Google Places API (New).
package goplaces

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// DefaultBaseURL is the default endpoint for the Places API (New).
const DefaultBaseURL = "https://places.googleapis.com/v1"

const (
	defaultSearchLimit  = 10
	defaultResolveLimit = 5
	maxSearchLimit      = 20
	maxResolveLimit     = 10
)

const (
	searchFieldMask  = "places.id,places.displayName,places.formattedAddress,places.location,places.rating,places.priceLevel,places.types,places.currentOpeningHours,nextPageToken"
	detailsFieldMask = "id,displayName,formattedAddress,location,rating,priceLevel,types,regularOpeningHours,currentOpeningHours,nationalPhoneNumber,websiteUri"
	resolveFieldMask = "places.id,places.displayName,places.formattedAddress,places.location,places.types"
)

const (
	priceLevelFree        = "PRICE_LEVEL_FREE"
	priceLevelInexpensive = "PRICE_LEVEL_INEXPENSIVE"
	priceLevelModerate    = "PRICE_LEVEL_MODERATE"
	priceLevelExpensive   = "PRICE_LEVEL_EXPENSIVE"
	priceLevelVeryExp     = "PRICE_LEVEL_VERY_EXPENSIVE"
)

var priceLevelToEnum = map[int]string{
	0: priceLevelFree,
	1: priceLevelInexpensive,
	2: priceLevelModerate,
	3: priceLevelExpensive,
	4: priceLevelVeryExp,
}

var enumToPriceLevel = map[string]int{
	priceLevelFree:        0,
	priceLevelInexpensive: 1,
	priceLevelModerate:    2,
	priceLevelExpensive:   3,
	priceLevelVeryExp:     4,
}

// Client wraps access to the Google Places API.
type Client struct {
	apiKey     string
	baseURL    string
	httpClient *http.Client
}

// Options configures the Places client.
type Options struct {
	APIKey     string
	BaseURL    string
	HTTPClient *http.Client
	Timeout    time.Duration
}

// NewClient builds a client with sane defaults.
func NewClient(opts Options) *Client {
	baseURL := strings.TrimRight(opts.BaseURL, "/")
	if baseURL == "" {
		baseURL = DefaultBaseURL
	}

	client := opts.HTTPClient
	if client == nil {
		timeout := opts.Timeout
		if timeout == 0 {
			timeout = 10 * time.Second
		}
		client = &http.Client{Timeout: timeout}
	}

	return &Client{
		apiKey:     opts.APIKey,
		baseURL:    baseURL,
		httpClient: client,
	}
}

// Search performs a text search with optional filters.
func (c *Client) Search(ctx context.Context, req SearchRequest) (SearchResponse, error) {
	req = applySearchDefaults(req)
	if err := validateSearchRequest(req); err != nil {
		return SearchResponse{}, err
	}

	body := buildSearchBody(req)
	payload, err := c.doRequest(ctx, http.MethodPost, "/places:searchText", body, searchFieldMask)
	if err != nil {
		return SearchResponse{}, err
	}

	var response searchResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return SearchResponse{}, fmt.Errorf("goplaces: decode search response: %w", err)
	}

	results := make([]PlaceSummary, 0, len(response.Places))
	for _, place := range response.Places {
		results = append(results, mapPlaceSummary(place))
	}

	return SearchResponse{
		Results:       results,
		NextPageToken: response.NextPageToken,
	}, nil
}

// Details fetches details for a specific place ID.
func (c *Client) Details(ctx context.Context, placeID string) (PlaceDetails, error) {
	placeID = strings.TrimSpace(placeID)
	if placeID == "" {
		return PlaceDetails{}, ValidationError{Field: "place_id", Message: "required"}
	}

	path := "/places/" + placeID
	payload, err := c.doRequest(ctx, http.MethodGet, path, nil, detailsFieldMask)
	if err != nil {
		return PlaceDetails{}, err
	}

	var place placeItem
	if err := json.Unmarshal(payload, &place); err != nil {
		return PlaceDetails{}, fmt.Errorf("goplaces: decode place details: %w", err)
	}

	return mapPlaceDetails(place), nil
}

// Resolve converts a free-form location string into candidate places.
func (c *Client) Resolve(ctx context.Context, req LocationResolveRequest) (LocationResolveResponse, error) {
	req = applyResolveDefaults(req)
	if err := validateResolveRequest(req); err != nil {
		return LocationResolveResponse{}, err
	}

	body := map[string]any{
		"textQuery": req.LocationText,
		"pageSize":  req.Limit,
	}

	payload, err := c.doRequest(ctx, http.MethodPost, "/places:searchText", body, resolveFieldMask)
	if err != nil {
		return LocationResolveResponse{}, err
	}

	var response searchResponse
	if err := json.Unmarshal(payload, &response); err != nil {
		return LocationResolveResponse{}, fmt.Errorf("goplaces: decode resolve response: %w", err)
	}

	results := make([]ResolvedLocation, 0, len(response.Places))
	for _, place := range response.Places {
		results = append(results, mapResolvedLocation(place))
	}

	return LocationResolveResponse{Results: results}, nil
}

func (c *Client) doRequest(
	ctx context.Context,
	method string,
	path string,
	body any,
	fieldMask string,
) ([]byte, error) {
	if strings.TrimSpace(c.apiKey) == "" {
		return nil, ErrMissingAPIKey
	}

	url := c.baseURL + path
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("goplaces: encode request: %w", err)
		}
		reader = bytes.NewReader(payload)
	}

	request, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		return nil, fmt.Errorf("goplaces: build request: %w", err)
	}

	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-Goog-Api-Key", c.apiKey)
	request.Header.Set("X-Goog-FieldMask", fieldMask)

	response, err := c.httpClient.Do(request)
	if err != nil {
		return nil, fmt.Errorf("goplaces: request failed: %w", err)
	}
	defer func() {
		_ = response.Body.Close()
	}()

	payload, err := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("goplaces: read response: %w", err)
	}

	if response.StatusCode >= http.StatusBadRequest {
		apiErr := &APIError{StatusCode: response.StatusCode, Body: strings.TrimSpace(string(payload))}
		return nil, apiErr
	}

	if len(payload) == 0 {
		return nil, errors.New("goplaces: empty response")
	}

	return payload, nil
}

func buildSearchBody(req SearchRequest) map[string]any {
	textQuery := req.Query
	if req.Filters != nil && strings.TrimSpace(req.Filters.Keyword) != "" {
		textQuery = strings.TrimSpace(textQuery + " " + req.Filters.Keyword)
	}

	body := map[string]any{
		"textQuery": textQuery,
		"pageSize":  req.Limit,
	}

	if req.PageToken != "" {
		body["pageToken"] = req.PageToken
	}

	if req.LocationBias != nil {
		body["locationBias"] = map[string]any{
			"circle": map[string]any{
				"center": map[string]any{
					"latitude":  req.LocationBias.Lat,
					"longitude": req.LocationBias.Lng,
				},
				"radius": req.LocationBias.RadiusM,
			},
		}
	}

	if req.Filters != nil {
		filters := req.Filters
		if len(filters.Types) > 0 {
			body["includedType"] = filters.Types[0]
		}
		if filters.OpenNow != nil {
			body["openNow"] = *filters.OpenNow
		}
		if filters.MinRating != nil {
			body["minRating"] = *filters.MinRating
		}
		if len(filters.PriceLevels) > 0 {
			levels := make([]string, 0, len(filters.PriceLevels))
			for _, level := range filters.PriceLevels {
				if mapped, ok := priceLevelToEnum[level]; ok {
					levels = append(levels, mapped)
				}
			}
			if len(levels) > 0 {
				body["priceLevels"] = levels
			}
		}
	}

	return body
}

func mapPlaceSummary(place placeItem) PlaceSummary {
	return PlaceSummary{
		PlaceID:    place.ID,
		Name:       displayName(place.DisplayName),
		Address:    place.FormattedAddress,
		Location:   mapLatLng(place.Location),
		Rating:     place.Rating,
		PriceLevel: mapPriceLevel(place.PriceLevel),
		Types:      place.Types,
		OpenNow:    openNow(place.CurrentOpeningHours),
	}
}

func mapPlaceDetails(place placeItem) PlaceDetails {
	return PlaceDetails{
		PlaceID:    place.ID,
		Name:       displayName(place.DisplayName),
		Address:    place.FormattedAddress,
		Location:   mapLatLng(place.Location),
		Rating:     place.Rating,
		PriceLevel: mapPriceLevel(place.PriceLevel),
		Types:      place.Types,
		Phone:      place.NationalPhoneNumber,
		Website:    place.WebsiteURI,
		Hours:      weekdayDescriptions(place.RegularOpeningHours),
		OpenNow:    openNow(place.CurrentOpeningHours),
	}
}

func mapResolvedLocation(place placeItem) ResolvedLocation {
	return ResolvedLocation{
		PlaceID:  place.ID,
		Name:     displayName(place.DisplayName),
		Address:  place.FormattedAddress,
		Location: mapLatLng(place.Location),
		Types:    place.Types,
	}
}

func mapLatLng(loc *location) *LatLng {
	if loc == nil {
		return nil
	}
	return &LatLng{Lat: loc.Latitude, Lng: loc.Longitude}
}

func displayName(name *displayNamePayload) string {
	if name == nil {
		return ""
	}
	return name.Text
}

func openNow(hours *openingHours) *bool {
	if hours == nil {
		return nil
	}
	return hours.OpenNow
}

func weekdayDescriptions(hours *openingHours) []string {
	if hours == nil {
		return nil
	}
	return hours.WeekdayDescriptions
}

func mapPriceLevel(value string) *int {
	if value == "" {
		return nil
	}
	if mapped, ok := enumToPriceLevel[value]; ok {
		return &mapped
	}
	return nil
}

func applySearchDefaults(req SearchRequest) SearchRequest {
	if req.Limit == 0 {
		req.Limit = defaultSearchLimit
	}
	return req
}

func applyResolveDefaults(req LocationResolveRequest) LocationResolveRequest {
	if req.Limit == 0 {
		req.Limit = defaultResolveLimit
	}
	return req
}

func validateSearchRequest(req SearchRequest) error {
	if strings.TrimSpace(req.Query) == "" {
		return ValidationError{Field: "query", Message: "required"}
	}
	if req.Limit < 1 || req.Limit > maxSearchLimit {
		return ValidationError{Field: "limit", Message: fmt.Sprintf("must be 1-%d", maxSearchLimit)}
	}

	if req.Filters != nil {
		if req.Filters.MinRating != nil {
			if *req.Filters.MinRating < 0 || *req.Filters.MinRating > 5 {
				return ValidationError{Field: "filters.min_rating", Message: "must be 0-5"}
			}
		}
		for _, level := range req.Filters.PriceLevels {
			if level < 0 || level > 4 {
				return ValidationError{Field: "filters.price_levels", Message: "must be 0-4"}
			}
		}
	}

	if req.LocationBias != nil {
		if err := validateLocationBias(req.LocationBias); err != nil {
			return err
		}
	}

	return nil
}

func validateResolveRequest(req LocationResolveRequest) error {
	if strings.TrimSpace(req.LocationText) == "" {
		return ValidationError{Field: "location_text", Message: "required"}
	}
	if req.Limit < 1 || req.Limit > maxResolveLimit {
		return ValidationError{Field: "limit", Message: fmt.Sprintf("must be 1-%d", maxResolveLimit)}
	}
	return nil
}

func validateLocationBias(bias *LocationBias) error {
	if bias == nil {
		return nil
	}
	if bias.RadiusM <= 0 {
		return ValidationError{Field: "location_bias.radius_m", Message: "must be > 0"}
	}
	if bias.Lat < -90 || bias.Lat > 90 {
		return ValidationError{Field: "location_bias.lat", Message: "must be -90..90"}
	}
	if bias.Lng < -180 || bias.Lng > 180 {
		return ValidationError{Field: "location_bias.lng", Message: "must be -180..180"}
	}
	return nil
}

type searchResponse struct {
	Places        []placeItem `json:"places"`
	NextPageToken string      `json:"nextPageToken"`
}

type placeItem struct {
	ID                  string              `json:"id"`
	DisplayName         *displayNamePayload `json:"displayName,omitempty"`
	FormattedAddress    string              `json:"formattedAddress,omitempty"`
	Location            *location           `json:"location,omitempty"`
	Rating              *float64            `json:"rating,omitempty"`
	PriceLevel          string              `json:"priceLevel,omitempty"`
	Types               []string            `json:"types,omitempty"`
	CurrentOpeningHours *openingHours       `json:"currentOpeningHours,omitempty"`
	RegularOpeningHours *openingHours       `json:"regularOpeningHours,omitempty"`
	NationalPhoneNumber string              `json:"nationalPhoneNumber,omitempty"`
	WebsiteURI          string              `json:"websiteUri,omitempty"`
}

type displayNamePayload struct {
	Text string `json:"text"`
}

type location struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type openingHours struct {
	OpenNow             *bool    `json:"openNow,omitempty"`
	WeekdayDescriptions []string `json:"weekdayDescriptions,omitempty"`
}
