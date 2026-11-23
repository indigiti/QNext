package marketcalendar

import (
 "testing"
 "time"
)
func TestOKXMarketCalendarNeverClosesOnWeekend(t *testing.T) {
 from:=time.Date(2026,11,21,0,0,0,0,time.UTC)
 to:=from.Add(72*time.Hour)
 windows,def,err:=DefaultRegistry().Windows("OKX_CRYPTO_24_7",from,to,SessionRegular)
 if err!=nil {t.Fatal(err)}
 if def.Timezone!="UTC"||len(windows)!=3 {t.Fatalf("expected three 24h windows, got %+v",windows)}
 for i,w:=range windows {
  if !w.Start.Equal(from.Add(time.Duration(i)*24*time.Hour))||w.End.Sub(w.Start)!=24*time.Hour {
   t.Fatalf("bad 24h window: %+v",w)
  }
 }
}
