package okx

import (
 "context"
 "fmt"
 "net/http"
 "net/http/httptest"
 "testing"
 "time"
)

func TestDiscoverXAUSwapExactLiveContract(t *testing.T) {
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  if r.URL.Query().Get("instType")!="SWAP"||r.URL.Query().Get("instId")!="XAU-USDT-SWAP"{
   t.Errorf("unexpected discovery query: %s",r.URL);return
  }
  fmt.Fprint(w,`{"code":"0","data":[{"instId":"XAU-USDT-SWAP","instType":"SWAP","state":"live","ctVal":"0.001","ctValCcy":"XAU","ctType":"linear","settleCcy":"USDT"}]}`)
 }))
 defer server.Close()
 c,err:=(Client{BaseURL:server.URL}).DiscoverXAUSwap(context.Background())
 if err!=nil{t.Fatal(err)}
 if c.InstrumentID!=SwapInstrumentID||c.InstID!=SwapProviderID||c.ContractValue!=0.001 {
  t.Fatalf("wrong swap %+v",c)
 }
}
func TestDiscoverXAUSwapFailsClosed(t *testing.T) {
 for _,variant:=range []struct{name,values string}{
  {"inactive",`"state":"suspend","ctValCcy":"XAU","ctType":"linear","settleCcy":"USDT"`},
  {"not_gold",`"state":"live","ctValCcy":"BTC","ctType":"linear","settleCcy":"USDT"`},
  {"inverse",`"state":"live","ctValCcy":"XAU","ctType":"inverse","settleCcy":"USDT"`},
  {"wrong_quote",`"state":"live","ctValCcy":"XAU","ctType":"linear","settleCcy":"USD"`},
 }{
  t.Run(variant.name,func(t *testing.T){
   server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
    fmt.Fprint(w,`{"code":"0","data":[{"instId":"XAU-USDT-SWAP","instType":"SWAP","ctVal":"0.001",`+variant.values+`}]}`)
   }))
   defer server.Close()
   if contract,err:=(Client{BaseURL:server.URL}).DiscoverXAUSwap(context.Background());err==nil{
    t.Fatalf("must reject %+v",contract)
   }
  })
 }
}
func TestSwapHistoryStoresSeparateCanonicalCandles(t *testing.T){
 now:=time.Date(2026,11,23,13,0,0,0,time.UTC)
 server:=httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter,r *http.Request){
  fmt.Fprintf(w,`{"code":"0","data":[["%d","3900","3910","3890","3905","40","0","0","1"]]}`,now.Add(-time.Minute).UnixMilli())
 }))
 defer server.Close()
 bars,err:=(Client{BaseURL:server.URL}).History(context.Background(),
  Contract{InstrumentID:SwapInstrumentID,InstID:SwapProviderID,ContractValue:0.001},
  "1m",now.Add(-2*time.Minute),now)
 if err!=nil{t.Fatal(err)}
 if len(bars)!=1 || bars[0].InstrumentID!=SwapInstrumentID || bars[0].Volume!=0.04 {
  t.Fatalf("swap bar mixed with X-Perp or wrongly converted %+v",bars)
 }
}
