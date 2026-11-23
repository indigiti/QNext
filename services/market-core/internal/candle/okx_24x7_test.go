package candle

import (
 "testing"
 "time"
 "github.com/indigiti/QNext/services/market-core/internal/domain"
)

func TestOKXGoldUTCWeekendAndMidnight(t *testing.T) {
 id:="OKX:XAUUSD-XPERP"
 at:=time.Date(2026,11,22,23,59,58,0,time.UTC) // Sunday
 if !InInstrumentSession(id,at){t.Fatal("OKX must trade on Sunday")}
 open,closeAt,err:=BucketForInstrument(id,at,"5s")
 if err!=nil {t.Fatal(err)}
 if open.Hour()!=23||open.Minute()!=59||open.Second()!=55||closeAt.Day()!=23||closeAt.Hour()!=0 {
  t.Fatalf("wrong UTC midnight bucket: %s .. %s",open,closeAt)
 }
 e:=New("test")
 if _,err:=e.Apply(domain.Tick{InstrumentID:id,Provider:"okx",Price:4000,Quantity:0.001,EventTime:at,Quality:domain.QualityGood},"5s");err!=nil {t.Fatal(err)}
 next:=at.Add(3*time.Second)
 bars,err:=e.Apply(domain.Tick{InstrumentID:id,Provider:"okx",Price:4001,Quantity:0.002,EventTime:next,Quality:domain.QualityGood},"5s")
 if err!=nil {t.Fatal(err)}
 if len(bars)!=2||!bars[0].Final||bars[0].Volume!=0.001||!bars[0].CloseTime.Equal(closeAt) {
  t.Fatalf("wrong 24x7 rollover: %+v",bars)
 }
}
