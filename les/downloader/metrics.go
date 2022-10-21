// Copyright 2015 The go-sdcereum Authors
// This file is part of the go-sdcereum library.
//
// The go-sdcereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-sdcereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-sdcereum library. If not, see <http://www.gnu.org/licenses/>.

// Contains the metrics collected by the downloader.

package downloader

import (
	"github.com/sdcereum/go-sdcereum/metrics"
)

var (
	headerInMeter      = metrics.NewRegisteredMeter("sdc/downloader/headers/in", nil)
	headerReqTimer     = metrics.NewRegisteredTimer("sdc/downloader/headers/req", nil)
	headerDropMeter    = metrics.NewRegisteredMeter("sdc/downloader/headers/drop", nil)
	headerTimeoutMeter = metrics.NewRegisteredMeter("sdc/downloader/headers/timeout", nil)

	bodyInMeter      = metrics.NewRegisteredMeter("sdc/downloader/bodies/in", nil)
	bodyReqTimer     = metrics.NewRegisteredTimer("sdc/downloader/bodies/req", nil)
	bodyDropMeter    = metrics.NewRegisteredMeter("sdc/downloader/bodies/drop", nil)
	bodyTimeoutMeter = metrics.NewRegisteredMeter("sdc/downloader/bodies/timeout", nil)

	receiptInMeter      = metrics.NewRegisteredMeter("sdc/downloader/receipts/in", nil)
	receiptReqTimer     = metrics.NewRegisteredTimer("sdc/downloader/receipts/req", nil)
	receiptDropMeter    = metrics.NewRegisteredMeter("sdc/downloader/receipts/drop", nil)
	receiptTimeoutMeter = metrics.NewRegisteredMeter("sdc/downloader/receipts/timeout", nil)

	stateInMeter   = metrics.NewRegisteredMeter("sdc/downloader/states/in", nil)
	stateDropMeter = metrics.NewRegisteredMeter("sdc/downloader/states/drop", nil)

	throttleCounter = metrics.NewRegisteredCounter("sdc/downloader/throttle", nil)
)
