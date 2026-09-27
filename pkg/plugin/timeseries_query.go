package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/grafana/grafana-plugin-sdk-go/backend"
	"github.com/grafana/grafana-plugin-sdk-go/backend/log"
)

type BatchSubRequest struct {
	Method     string            `json:"Method"`
	Resource   string            `json:"Resource"`
	ParentIds  []string          `json:"ParentIds"`
	Parameters []string          `json:"Parameters"`
	Headers    map[string]string `json:"Headers"`
}

type BatchSubRequestMap map[string]BatchSubRequest

// processQuery is the main function for processing queries. It takes a query and returns a slice of PiProcessedQuery
// that contains batched queries that are ready to be sent to the PI Web API.
// A query that cannot be processed gets a PiProcessedQuery with its RefID and the error, which is reported on
// that query's response; the other queries are processed normally.
func (d *Datasource) processQuery(allQueries []backend.DataQuery, datasourceUID string) []PiProcessedQuery {
	var ProcessedQuery []PiProcessedQuery

	index := 0
	for _, query := range allQueries {
		var PiQuery Query

		// Unmarshal the query into a PiQuery struct, and then unmarshal the PiQuery into a PiProcessedQuery
		// if there are errors we'll set the error and return the PiProcessedQuery with an error set.
		invalid := func(err error) {
			ProcessedQuery = append(ProcessedQuery, PiProcessedQuery{RefID: query.RefID, Error: err, Status: http.StatusBadRequest})
		}
		tempJson, err := json.Marshal(query)
		if err != nil {
			log.DefaultLogger.Error("Process query - Error marshalling", "error", err)
			invalid(fmt.Errorf("error while processing the query"))
			continue
		}

		err = json.Unmarshal(tempJson, &PiQuery)
		if err != nil {
			log.DefaultLogger.Error("Process query - Error unmarshalling", "error", err, "json", string(tempJson))
			invalid(fmt.Errorf("error while processing the query: %w", err))
			continue
		}

		// Determine if we are using units in the response.
		// The front end doesn't guarantee that the UseUnit field will be set, so we need to check for nils
		var UseUnit = false
		if PiQuery.Pi.UseUnit != nil && PiQuery.Pi.UseUnit.Enable != nil {
			if *PiQuery.Pi.UseUnit.Enable {
				UseUnit = true
			}
		}
		var DigitalStates = false
		if PiQuery.Pi.DigitalStates != nil && PiQuery.Pi.DigitalStates.Enable != nil {
			if *PiQuery.Pi.DigitalStates.Enable {
				DigitalStates = true
			}
		}

		// Upon creating a dashboard the initial query will be empty, so we need to check for that to avoid errors
		// if the query is empty, we'll return a PiProcessedQuery with an error set.
		err = PiQuery.isValidQuery()
		if err != nil {
			invalid(err)
			continue
		}

		// At this point we expect that the query is valid, so we can start processing it.
		// Multi-value template variables in the element path and in the attributes are expanded
		// into every element/attribute combination.
		targets, err := PiQuery.Pi.getExpandedTargets()
		if err != nil {
			log.DefaultLogger.Warn("Process query - Error expanding template variables", "RefID", PiQuery.RefID, "error", err)
			invalid(err)
			continue
		}

		for _, target := range targets {
			targetBasePath := target.BasePath
			fullTargetPath := targetBasePath + PiQuery.Pi.getTargetPathSeparator() + target.Attribute
			// Index is unique within the request and is used to build the batch request keys
			index++
			// Create a processed query for the target
			piQuery := PiProcessedQuery{
				RefID:               PiQuery.RefID,
				Label:               target.Attribute,
				UID:                 datasourceUID,
				IntervalNanoSeconds: PiQuery.Interval,
				IsPIPoint:           PiQuery.Pi.IsPiPoint,
				HideError:           PiQuery.Pi.HideError,
				Streamable:          PiQuery.isStreamable() && d.isUsingStreaming(),
				FullTargetPath:      fullTargetPath,
				TargetPath:          targetBasePath,
				UseUnit:             UseUnit,
				DigitalStates:       DigitalStates,
				Display:             PiQuery.Pi.Display,
				Regex:               PiQuery.Pi.Regex,
				Nodata:              PiQuery.Pi.Nodata,
				Summary:             PiQuery.Pi.Summary,
				HashCode:            PiQuery.Pi.HashCode + "_" + fullTargetPath,
				StartTime:           PiQuery.TimeRange.From.Truncate(time.Second),
				EndTime:             PiQuery.TimeRange.To.Truncate(time.Second),
				Variable:            target.Variable,
				MultiVariable:       target.MultiVariable,
				Index:               index,
			}

			WebID := d.getCachedWebID(fullTargetPath)

			// initialize maps
			piQuery.BatchRequest = make(map[string]BatchSubRequest)

			var baseUrl = d.settings.URL
			if !strings.HasSuffix(baseUrl, "/") {
				baseUrl = baseUrl + "/"
			}
			dataId := fmt.Sprintf("%s_Req%d_Data", piQuery.RefID, piQuery.Index)
			if WebID != nil && WebID.WebID != "" {
				piQuery.WebID = WebID.WebID
				// DATA FETCH
				batchSubRequest := BatchSubRequest{
					Method:   "GET",
					Resource: baseUrl + PiQuery.getQueryBaseURL() + WebID.WebID,
					Headers: map[string]string{
						"Asset-Path": fullTargetPath,
					},
				}
				piQuery.Resource = batchSubRequest.Resource
				piQuery.BatchRequest[dataId] = batchSubRequest
			} else {
				parentId := fmt.Sprintf("%s_Req%d", piQuery.RefID, piQuery.Index)
				parameter := "$." + parentId + ".Content.WebId"
				// WEBID FETCH
				piQuery.BatchRequest[parentId] = BatchSubRequest{
					Method:   "GET",
					Resource: baseUrl + d.getRequestWebId(fullTargetPath, piQuery.IsPIPoint),
				}
				// DATA FETCH
				batchSubRequest := BatchSubRequest{
					Method:     "GET",
					ParentIds:  []string{parentId},
					Parameters: []string{parameter},
					Resource:   baseUrl + PiQuery.getQueryBaseURL() + "{0}",
				}
				piQuery.Resource = batchSubRequest.Resource
				piQuery.BatchRequest[dataId] = batchSubRequest
			}

			ProcessedQuery = append(ProcessedQuery, piQuery)
		}
	}

	return ProcessedQuery
}

// batchRequest sends the processed queries to the PI Web API and groups them by RefID. Queries that already have
// an error (they could not be processed) are not sent, and are returned with their error.
func (d *Datasource) batchRequest(ctx context.Context, PIWebAPIQueriesAll []PiProcessedQuery) map[string][]PiProcessedQuery {
	valid := make([]PiProcessedQuery, 0, len(PIWebAPIQueriesAll))
	var invalid []PiProcessedQuery
	for _, piQuery := range PIWebAPIQueriesAll {
		if piQuery.Error != nil {
			invalid = append(invalid, piQuery)
		} else {
			valid = append(valid, piQuery)
		}
	}
	PIWebAPIQueries := d.sendBatch(ctx, valid)
	for _, piQuery := range invalid {
		PIWebAPIQueries[piQuery.RefID] = append(PIWebAPIQueries[piQuery.RefID], piQuery)
	}
	return PIWebAPIQueries
}

func (d *Datasource) sendBatch(ctx context.Context, PIWebAPIQueriesAll []PiProcessedQuery) map[string][]PiProcessedQuery {
	batchRequest := make(map[string]BatchSubRequest)
	PIWebAPIQueries := make(map[string][]PiProcessedQuery)
	// create a map of the batch requests. This allows us to map the response back to the original query
	for _, piQuery := range PIWebAPIQueriesAll {
		for key, request := range piQuery.BatchRequest {
			batchRequest[key] = request
		}
		piQuery.Cached = false
		PIWebAPIQueries[piQuery.RefID] = append(PIWebAPIQueries[piQuery.RefID], piQuery)
	}

	// request the data from the PI Web API
	batchRequestResponse, err := apiBatchRequest(ctx, d, batchRequest)

	// process response
	if err != nil {
		for RefID, processedQuery := range PIWebAPIQueries {
			backend.Logger.Error("Batch request", "RefID", RefID, "error", err)
			for i, query := range processedQuery {
				if data, found := d.webCache.Get(query.HashCode); d.isUsingResponseCache() && found {
					log.DefaultLogger.Debug("Batch request - Cache get operation", "RefID", RefID, "index", query.Index)
					PIWebAPIQueries[RefID][i].Response = data
					PIWebAPIQueries[RefID][i].Status = http.StatusOK
					PIWebAPIQueries[RefID][i].Cached = true
				} else {
					PIWebAPIQueries[RefID][i].Error = fmt.Errorf("error during query: %s", err.Error())
					PIWebAPIQueries[RefID][i].Status = http.StatusBadGateway
				}
			}
		}
		return PIWebAPIQueries
	}

	tempresponse := make(map[string]PIBatchResponse)
	err = json.Unmarshal(batchRequestResponse, &tempresponse)
	if err != nil {
		for RefID, processedQuery := range PIWebAPIQueries {
			backend.Logger.Error("Batch request - Unmarshal", "RefID", RefID, "error", err, "tempresponse", tempresponse)
			for i := range processedQuery {
				PIWebAPIQueries[RefID][i].Error = fmt.Errorf("error during query. bad response format")
				PIWebAPIQueries[RefID][i].Status = http.StatusInternalServerError
			}
		}
		return PIWebAPIQueries
	}

	for RefID, processedQuery := range PIWebAPIQueries {
		// map the response back to the original query
		for i, query := range processedQuery {
			// WEBID
			var key = fmt.Sprintf("%s_Req%d", RefID, query.Index)
			WebIdData, ok := tempresponse[key]
			if ok {
				if WebIdData.Status == http.StatusOK {
					PIWebAPIQueries[RefID][i].WebID = d.saveWebID(WebIdData.Content, query.FullTargetPath, query.IsPIPoint)
				} else {
					backend.Logger.Error("Batch request - request bad", "Content", WebIdData.Content)
					jWebIdData, err := json.Marshal(WebIdData.Content)
					if err != nil {
						PIWebAPIQueries[RefID][i].Error = err
						continue
					}
					var errorResponse PiBatchDataError
					err = json.Unmarshal(jWebIdData, &errorResponse)
					if err != nil {
						PIWebAPIQueries[RefID][i].Error = err
						continue
					}
					if errorResponse.Error != nil && len(errorResponse.Error.Errors) > 0 {
						PIWebAPIQueries[RefID][i].Error = fmt.Errorf("api error %d - %s", WebIdData.Status, errorResponse.Error.Errors[0])
					} else {
						PIWebAPIQueries[RefID][i].Error = fmt.Errorf("unknown api error")
					}
					continue
				}
			}
			// DATA
			key = fmt.Sprintf("%s_Req%d_Data", RefID, query.Index)
			ResponseData, ok := tempresponse[key]
			if ok {
				if ResponseData.Status == http.StatusOK {
					PIWebAPIQueries[RefID][i].Response = ResponseData.Content.(PiBatchData)
					PIWebAPIQueries[RefID][i].Status = ResponseData.Status
					if d.isUsingResponseCache() {
						log.DefaultLogger.Debug("Batch request - Cache set", "RefID", RefID, "index", query.Index)
						d.webCache.Set(query.HashCode, PIWebAPIQueries[RefID][i].Response)
					}
				} else if data, found := d.webCache.Get(query.HashCode); d.isUsingResponseCache() && found {
					log.DefaultLogger.Debug("Batch request - Cache get", "RefID", RefID, "index", query.Index)
					PIWebAPIQueries[RefID][i].Response = data
					PIWebAPIQueries[RefID][i].Status = http.StatusOK
					PIWebAPIQueries[RefID][i].Cached = true
				} else {
					backend.Logger.Error("Batch request - bad", "Content", ResponseData.Content)
					d.webCache.Remove(query.HashCode)
					PIWebAPIQueries[RefID][i].Status = ResponseData.Status
					jResponseData, err := json.Marshal(ResponseData.Content)
					if err != nil {
						PIWebAPIQueries[RefID][i].Error = err
						continue
					}
					var errorResponse PiBatchDataError
					err = json.Unmarshal(jResponseData, &errorResponse)
					if err != nil {
						PIWebAPIQueries[RefID][i].Error = err
						continue
					}
					if errorResponse.Error != nil && len(errorResponse.Error.Errors) > 0 {
						PIWebAPIQueries[RefID][i].Error = fmt.Errorf("api error %d - %s", WebIdData.Status, errorResponse.Error.Errors[0])
					} else {
						PIWebAPIQueries[RefID][i].Error = fmt.Errorf("unknown api error")
					}
				}
			} else if data, found := d.webCache.Get(query.HashCode); d.isUsingResponseCache() && found {
				log.DefaultLogger.Debug("Batch request - Cache get", "RefID", RefID, "index", query.Index)
				PIWebAPIQueries[RefID][i].Response = data
				PIWebAPIQueries[RefID][i].Status = http.StatusOK
				PIWebAPIQueries[RefID][i].Cached = true
			} else {
				d.webCache.Remove(query.HashCode)
				PIWebAPIQueries[RefID][i].Error = fmt.Errorf("error finding key %s in response", key)
				PIWebAPIQueries[RefID][i].Status = http.StatusInternalServerError
			}
		}
	}

	return PIWebAPIQueries
}

/// END NEW

func (d *Datasource) processBatchtoFrames(processedQuery map[string][]PiProcessedQuery) *backend.QueryDataResponse {
	response := backend.NewQueryDataResponse()

	for RefID, query := range processedQuery {
		var subResponse backend.DataResponse
		var errorStatus backend.Status
		for _, q := range query {
			// A failing target (e.g. an element of a multi-value variable without the attribute) reports its
			// error and the other targets of the query still return their data.
			if q.Error != nil {
				backend.Logger.Error("Process batch to frames - Error processing query", "RefID", RefID, "query", q, "hide", q.HideError)
				if !q.HideError && subResponse.Error == nil {
					subResponse.Error = q.Error
					errorStatus = backend.Status(q.Status)
				}
				continue
			}
			subResponse.Status = backend.Status(q.Status)

			for _, SummaryType := range *q.Response.getSummaryTypes() {
				frame, err := convertItemsToDataFrame(&q, d, SummaryType)

				// if there is an error on a single frame we set metadata and continue to the next frame
				if err != nil {
					backend.Logger.Error("Process batch to frames - convertItemsToDataFrame", "RefID", RefID, "query", q)
					subResponse.Error = q.Error
					continue
				}

				frame.RefID = RefID
				// meta data
				frame.Meta.ExecutedQueryString = strings.ReplaceAll(q.Resource, "{0}", q.WebID)

				// TODO: enable streaming
				// If the query is streamable, then we need to set the channel URI
				// and the executed query string.
				if q.Streamable {
					// Create a new channel for this frame request.
					// Creating a new channel for each frame request is not ideal,
					// but it is the only way to ensure that the frame data is refreshed
					// on a time interval update.
					channeluuid := uuid.New()
					channelURI := "ds/" + q.UID + "/" + channeluuid.String()
					channel := StreamChannelConstruct{
						WebID:               q.WebID,
						IntervalNanoSeconds: q.IntervalNanoSeconds,
						tagLabel:            q.Label,
						query:               &q,
					}
					d.channelConstruct[channeluuid.String()] = channel
					frame.Meta.Channel = channelURI
				}

				subResponse.Frames = append(subResponse.Frames, frame)
			}
		}
		if len(subResponse.Frames) == 0 && errorStatus != 0 {
			subResponse.Status = errorStatus
		}
		response.Responses[RefID] = subResponse
	}
	return response
}

func (q *PIWebAPIQuery) isSummary() bool {
	if q.Summary == nil {
		return false
	}
	if q.Summary.Enable == nil || q.Summary.Basis == nil || q.Summary.Types == nil {
		return false
	}
	return *q.Summary.Enable && *q.Summary.Basis != "" && len(*q.Summary.Types) > 0
}

// PiProcessedQuery isRegex returns true if the query is a regex query and is enabled
func (q *PiProcessedQuery) isRegex() bool {
	if q.Regex == nil {
		return false
	}
	if q.Regex.Enable == nil {
		return false
	}
	return *q.Regex.Enable
}

// PiProcessedQuery isRegexValid returns true if the regex query is valid and enabled
func (q *PiProcessedQuery) isRegexQuery() bool {
	if !q.isRegex() {
		return false
	}
	if q.Regex.Replace == nil {
		return false
	}
	if q.Regex.Search == nil {
		return false
	}
	if len(*q.Regex.Replace) == 0 {
		return false
	}
	if len(*q.Regex.Search) == 0 {
		return false
	}
	return true
}

// getSummaryDuration returns the summary duration in the format piwebapi expects
// The summary duration is provided by the frontend in the format: <number><short_name>
// The short name can be one of the following: ms, s, m, h, d, mo, w, wd, yd
// A default of 30s is returned if the summary duration is not provided by the frontend
// or if the format is invalid
func (q *PIWebAPIQuery) getSummaryDuration() string {
	// Return the default value if the summary is not provided by the frontend
	if q.Summary == nil || q.Summary.Duration == nil || *q.Summary.Duration == "" {
		return "30s"
	}
	return _getDurationBase(*q.Summary.Duration)
}

func (q *PIWebAPIQuery) getSampleInterval() string {
	// Return the default value if the summary is not provided by the frontend
	if q.Summary == nil || q.Summary.SampleInterval == nil || *q.Summary.SampleInterval == "" {
		return "30s"
	}
	return _getDurationBase(*q.Summary.SampleInterval)
}

func _getDurationBase(duration string) string {
	// If the summary duration is provided, then validate the format piwebapi expects
	// Regular expression to match the format: <number><short_name>
	pattern := `^(\d+(\.\d+)?)\s*(ms|s|m|h|d|mo|w|wd|yd)$`
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(duration)

	if len(matches) != 4 {
		return "30s" // Return the default value if the format is invalid
	}

	// Extract the numeric part and the short name from the interval
	numericPartStr := matches[1]
	shortName := matches[3]

	// Convert the numeric part to a float64
	numericPart, err := strconv.ParseFloat(numericPartStr, 64)
	if err != nil {
		return "30s" // Return the default value if conversion fails
	}

	// Check if the short name is valid and whether fractions are allowed for that time unit
	switch shortName {
	case "ms", "s", "m", "h":
		// Fractions allowed for millisecond, second, minute, and hour
		return duration
	case "d", "mo", "w", "wd", "yd":
		// No fractions allowed for day, month, week, weekday, yearday
		if numericPart == float64(int64(numericPart)) {
			return duration
		}
	default:
		return "30s" // Return the default value if the short name or fractions are not allowed
	}

	return "30s" // Return the default value if the short name or fractions are not allowed
}

func (q *PIWebAPIQuery) getSummaryURIComponent() string {
	if !q.isSummary() {
		return ""
	}
	uri := ""
	for _, t := range *q.Summary.Types {
		uri += "&summaryType=" + t.Value.Value
	}
	uri += "&calculationBasis=" + *q.Summary.Basis
	if q.Summary.Duration != nil && *q.Summary.Duration != "" {
		uri += "&summaryDuration=" + q.getSummaryDuration()
	}
	if q.Summary.SampleTypeInterval != nil && *q.Summary.SampleTypeInterval &&
		q.Summary.SampleInterval != nil && *q.Summary.SampleInterval != "" {
		uri += "&sampleType=Interval&sampleInterval=" + q.getSampleInterval()
	}
	return uri
}

func (q *PIWebAPIQuery) isRecordedValues() bool {
	if q.RecordedValues == nil {
		return false
	}
	if q.RecordedValues.Enable == nil {
		return false
	}
	return *q.RecordedValues.Enable
}

func (q *PIWebAPIQuery) isInterpolated() bool {
	return q.Interpolate.Enable
}

func (q *PIWebAPIQuery) isExpression() bool {
	return q.Expression != ""
}

func (q *PIWebAPIQuery) getBasePath() string {
	if q.Target == nil {
		return ""
	}
	semiIndex := strings.Index(*q.Target, ";")
	if semiIndex == -1 {
		return *q.Target
	}
	return (*q.Target)[:semiIndex]
}

// func (q *PIWebAPIQuery) getfullTargetPath(target string) string {
// 	fullTargetPath := q.getBasePath()
// 	if q.IsPiPoint {
// 		fullTargetPath += `\` + target
// 	} else {
// 		fullTargetPath += "|" + target
// 	}
// 	return fullTargetPath
// }

func (q *PIWebAPIQuery) getTargetPathSeparator() string {
	if q.IsPiPoint {
		return `\`
	}
	return "|"
}

// func (q *PIWebAPIQuery) getTargets() []string {
// 	if q.Target == nil {
// 		return nil
// 	}

// 	semiIndex := strings.Index(*q.Target, ";")
// 	if semiIndex == -1 || semiIndex == len(*q.Target)-1 {
// 		return nil
// 	}
// 	return strings.Split((*q.Target)[semiIndex+1:], ";")
// }

func (q *PIWebAPIQuery) checkNilSegments() bool {
	return q.Target == nil
}

func (q *PIWebAPIQuery) checkValidTargets() bool {
	if q.Target == nil {
		return false
	}

	// check if the target provided is just a semicolon
	if strings.Compare(*q.Target, ";") == 0 {
		return false
	}
	// check if the target provided ends with a semicolon
	if q.Target == nil || strings.HasSuffix(*q.Target, ";") {
		return false
	}

	return true
}

func (q *PIWebAPIQuery) isUseLastValue() bool {
	if q.UseLastValue == nil {
		return false
	}
	if q.UseLastValue.Enable == nil {
		return false
	}
	return *q.UseLastValue.Enable
}

func (q *Query) getMaxDataPoints() int {
	if q.Pi.RecordedValues != nil && q.Pi.RecordedValues.MaxNumber != nil {
		return *q.Pi.RecordedValues.MaxNumber
	}
	return q.MaxDataPoints
}

func (q *Query) getBoundaryType() string {
	if q.Pi.RecordedValues != nil && q.Pi.RecordedValues.BoundaryType != nil {
		return *q.Pi.RecordedValues.BoundaryType
	}
	return "Inside"
}

func (q Query) getQueryBaseURL() string {
	var uri string
	if q.Pi.isExpression() {
		uri += "calculation"
		if q.Pi.isUseLastValue() {
			uri += "/times?time=" + q.getTimeRangeURIToComponent()
		} else {
			if q.Pi.isSummary() {
				uri += "/summary" + q.getTimeRangeURIComponent() + q.Pi.getSummaryURIComponent()
			} else if q.Pi.isInterpolated() {
				uri += "/intervals" + q.getTimeRangeURIComponent()
				uri += fmt.Sprintf("&sampleInterval=%s", q.getIntervalTime())
			} else if q.Pi.isRecordedValues() {
				uri += "/recorded" + q.getTimeRangeURIComponent()
			} else {
				// uri += "/times?" + q.getWindowedTimeStampURI()
				uri += "/recorded" + q.getTimeRangeURIComponent()
			}
		}
		uri += "&expression=" + queryEscape(q.Pi.Expression) + "&webId="
		log.DefaultLogger.Debug("Calculation log", "uri", uri)
	} else {
		uri += "streamsets"
		if q.Pi.isUseLastValue() {
			if q.Pi.isRecordedValues() {
				uri += "/end?webId="
			} else {
				uri += "/value?time=" + q.getTimeRangeURIToComponent() + "&webId="
			}
		} else {
			if q.Pi.isSummary() {
				uri += "/summary" + q.getTimeRangeURIComponent() + q.Pi.getSummaryURIComponent()
			} else if q.Pi.isInterpolated() {
				uri += "/interpolated" + q.getTimeRangeURIComponent() + fmt.Sprintf("&interval=%s", q.getIntervalTime())
			} else if q.Pi.isRecordedValues() {
				uri += "/recorded" + q.getTimeRangeURIComponent() + fmt.Sprintf("&maxCount=%d", q.getMaxDataPoints()) + "&boundaryType=" + q.getBoundaryType()
			} else {
				uri += "/plot" + q.getTimeRangeURIComponent() + fmt.Sprintf("&intervals=%d", q.getMaxDataPoints())
			}
			uri += "&webId="
		}
	}
	return uri
}
