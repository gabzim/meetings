package calendarsync

import "google.golang.org/api/calendar/v3"

func SyncEvent(e *calendar.Event, srv *calendar.Service) {
	srv.Events.Insert("id", e)
}
