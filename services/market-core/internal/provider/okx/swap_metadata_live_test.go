package okx

import (
 "context"
 "net/url"
 "os"
 "testing"
	"github.com/indigiti/QNext/services/market-core/internal/domain"
 "time"
)

func TestLiveOKXSwapMetadata(t *testing.T) {
 if os.Getenv("QNEXT_OKX_SWAP_SMOKE")!="1" {t.Skip("enable swap external test")}
 ctx,cancel:=context.WithTimeout(context.Background(),20*time.Second)
 defer cancel()
 var result apiResponse[struct {
  InstID string `json:"instId"`
  InstType string `json:"instType"`
  State string `json:"state"`
  CtVal string `json:"ctVal"`
  CtValCcy string `json:"ctValCcy"`
  CtType string `json:"ctType"`
  SettleCcy string `json:"settleCcy"`
  Uly string `json:"uly"`
 }]
 if err:=(Client{}).get(ctx,"/api/v5/public/instruments",url.Values{"instType":{"SWAP"},"instId":{"XAU-USDT-SWAP"}},&result);err!=nil{t.Fatal(err)}
 t.Logf("OKX SWAP code=%s msg=%s entries=%d",result.Code,result.Msg,len(result.Data))
 for _,s:=range result.Data{t.Logf("SWAP instId=%s instType=%s state=%s ctVal=%s ctValCcy=%s ctType=%s settleCcy=%s uly=%s",s.InstID,s.InstType,s.State,s.CtVal,s.CtValCcy,s.CtType,s.SettleCcy,s.Uly)}
 if result.Code!="0"||len(result.Data)!=1||result.Data[0].InstID!="XAU-USDT-SWAP"{t.Fatal("exact active XAU-USDT-SWAP metadata unavailable")}
}

func TestLiveOKXSwapFeed(t *testing.T) {
 if os.Getenv("QNEXT_OKX_SWAP_SMOKE")!="1"{t.Skip("external API check disabled")}
 ctx,cancel:=context.WithTimeout(context.Background(),65*time.Second)
 defer cancel()
 client:=Client{}
 contract,err:=client.DiscoverXAUSwap(ctx)
 if err!=nil{t.Fatal(err)}
 t.Logf("verified live swap contract %s ctVal=%g XAU",contract.InstID,contract.ContractValue)
 now:=time.Now().UTC()
 minute,err:=client.History(ctx,contract,"1m",now.Add(-45*time.Minute),now.Add(-2*time.Minute))
 if err!=nil || len(minute)==0 {t.Fatalf("swap 1m history unavailable: %d, %v",len(minute),err)}
 second,err:=client.History(ctx,contract,"5s",now.Add(-2*time.Minute),now.Add(-30*time.Second))
 if err!=nil{t.Fatalf("swap 1s/5s history failure: %v",err)}
 t.Logf("swap finalized 1m=%d 5s-contiguous=%d",len(minute),len(second))
 runCtx,stop:=context.WithTimeout(ctx,25*time.Second);defer stop()
 ack:=make(chan struct{},1);got:=make(chan domain.Tick,1);done:=make(chan error,1)
 go func(){done<- (Stream{}).once(runCtx,contract,func(_ string,tick domain.Tick)error{
  select{case got<-tick:default:};return nil
 },func(){select{case ack<-struct{}{}:default:}})}()
 select{
 case <-ack:t.Log("swap trades-all acknowledged")
 case err:=<-done:t.Fatalf("swap subscription ended: %v",err)
 case <-runCtx.Done():t.Fatal("no swap subscription ACK")
 }
 select{
 case tick:=<-got:
  if tick.InstrumentID!=SwapInstrumentID||tick.Price<=0||tick.Quantity<=0 {t.Fatalf("bad swap tick %+v",tick)}
  t.Logf("swap trade price=%f qtyXAU=%f time=%s",tick.Price,tick.Quantity,tick.TradeTime)
 case err:=<-done:t.Fatalf("swap stream exited: %v",err)
 case <-runCtx.Done():t.Fatal("no swap trade received in 25s")
 }
 stop()
 select{case <-done:case <-time.After(2*time.Second):t.Fatal("swap stream cancellation failure")}
}
