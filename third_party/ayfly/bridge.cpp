#include <stdint.h>
#include <stddef.h>
extern "C" {
#include "../pt3/ayumi.h"
void *memset(void *,int,size_t);
void *memcpy(void *,const void *,size_t);
int abs(int);
}
static size_t strlen(const char *s){size_t n=0;while(s[n])n++;return n;}
static int strncmp(const char *a,const char *b,size_t n){for(size_t i=0;i<n;i++){if(a[i]!=b[i])return (unsigned char)a[i]-(unsigned char)b[i];if(!a[i])break;}return 0;}
struct AYSongInfo {unsigned char *module; void *data; bool ended; uint8_t regs[14];};
enum {AY_CHNL_A_FINE,AY_CHNL_A_COARSE,AY_CHNL_B_FINE,AY_CHNL_B_COARSE,AY_CHNL_C_FINE,AY_CHNL_C_COARSE,AY_NOISE_PERIOD,AY_MIXER,AY_CHNL_A_VOL,AY_CHNL_B_VOL,AY_CHNL_C_VOL,AY_ENV_FINE,AY_ENV_COARSE,AY_ENV_SHAPE};
unsigned short ay_sys_getword(unsigned char *p){return p[0]|(p[1]<<8);}
void ay_sys_writeword(unsigned char *p,unsigned short v){p[0]=v;p[1]=v>>8;}
void ay_writeay(AYSongInfo *i,int r,unsigned char v){i->regs[r]=v;}
void ay_resetay(AYSongInfo *i,int){memset(i->regs,0,14);i->regs[13]=255;}
static int checked_offset(long i){if(i<0||i>65536+1024)__builtin_trap();return (int)i;}
static unsigned note_index(int i,unsigned n){return i<0?0:(unsigned)i>=n?n-1:(unsigned)i;}
unsigned char ay_readay(AYSongInfo *i,int r){return i->regs[r];}
#include "tables.h"
#include "PT2Play.h"
#include "STPPlay.h"
#include "ASCPlay.h"
#include "PSCPlay.h"
#include "SQTPlay.h"
#include "STCPlay.h"
#include "PT1Play.h"
#include "registers.h"
static constexpr int MAX_DUMP_INPUT=2*1024*1024,MAX_DUMP_OUTPUT=4*1024*1024;
static uint8_t input[MAX_DUMP_INPUT],module[MAX_DUMP_OUTPUT];
static Dump dump;
static unsigned tickHz,frameIndex;
static float output[32768];
static AYSongInfo song;
static ayumi chip;
static int rate,remaining,format,elapsed;
static int streamPos,streamLen,streamSkip;
extern "C" {
uint8_t* rr_input(){return input;}
float* rr_output(){return output;}
int rr_ended(){return song.ended;}
int rr_init(int len,int sr,int fmt){
 if(len<16||len>(fmt>=10?MAX_DUMP_INPUT:65536)||sr<22050||sr>192000||(fmt<1||fmt>12||fmt==3))return 0;
 memset(module,0,fmt>=10?sizeof(module):131072);memcpy(module,input,len);song={};song.module=module;tickHz=50;frameIndex=0;unsigned clock=1773400,ym=0;
 if(fmt==2){
  if(len<133||!module[0])return 0;
  const int pat=ay_sys_getword(module+99);
  if(pat<132||pat+6>len||module[pat-1]!=255)return 0;
  int count=0;while(131+count<pat&&module[131+count]<128)count++;
  if(!count||count>255||module[2]>=count||131+count!=pat-1)return 0;
  for(int i=0;i<count;i++){int off=pat+module[131+i]*6;if(off+6>len)return 0;for(int j=0;j<3;j++){int ptr=ay_sys_getword(module+off+j*2);if(ptr<pat||ptr>=len)return 0;}}
  for(int i=0;i<48;i++){int ptr=ay_sys_getword(module+3+i*2);if(ptr&&ptr+2>len)return 0;}
  module[1]=count;PT2_Init(song);
 }
 else if(fmt==4){if(!module[0]||!STP_Detect(module,len))return 0;STP_Init(song);}
 else if(fmt==1){if(!module[0]||!PT1_Detect(module,len)||!module[1]||module[2]>=module[1])return 0;PT1_Init(song);}
 else if(fmt==5||fmt==6){
  // The original detector rejects valid modules with unused, omitted final ornaments.
  auto validASC=[&](bool old){
   int shift=old?0:1,head=8+shift;
   if(len<head||!module[0])return false;
   int p=ay_sys_getword(module+1+shift),s=ay_sys_getword(module+3+shift),o=ay_sys_getword(module+5+shift),n=module[7+shift];
   if(!n||(!old&&module[1]>=n)||p<head+n||p+6>s||s+64>o||o+64>len||ay_sys_getword(module+s)!=64||ay_sys_getword(module+o)!=64)return false;
   int table=ay_sys_getword(module+p);
   if(table<6||table%6||p+table>s)return false;
   for(int i=0;i<n;i++){int off=module[head+i]*6;if(off+6>table)return false;for(int c=0;c<3;c++){int addr=p+ay_sys_getword(module+p+off+c*2);if(addr<p+table||addr>=s)return false;}}
   return true;
  };
  if(fmt==6||!validASC(false)){
   if(len>=65536||!validASC(true))return 0;
   for(int i=len;i>1;--i)module[i]=module[i-1];module[1]=0;++len;
   for(int i=2;i<=6;i+=2)ay_sys_writeword(module+i,ay_sys_getword(module+i)+1);
  }
  if(!validASC(false))return 0;ASC_Init(song);
 }
 else if(fmt==7){if(!PSC_Detect(module,len)||!module[73])return 0;PSC_Init(song);}
 else if(fmt==8){if(!SQT_Detect(module,len))return 0;SQT_Init(song);if(!song.data)return 0;}
 else if(fmt==9){if(!module[0]||!STC_Detect(module,len))return 0;STC_Init(song);if(song.ended)return 0;}
 else if(fmt==10){if(len<17||module[0]!='P'||module[1]!='S'||module[2]!='G'||module[3]!=26)return 0;streamPos=16;streamLen=len;streamSkip=0;ay_resetay(&song,0);}

 else if(fmt==11||fmt==12){
  if(!(fmt==11?decodeYM(input,len,module,sizeof(module),dump):decodeVTX(input,len,module,sizeof(module),dump)))return 0;
  if(dump.hz<25||dump.hz>200||dump.clock<500000||dump.clock>4000000||!dump.count)return 0;
  tickHz=dump.hz;clock=dump.clock;ym=dump.ym;ay_resetay(&song,0);
 }
 if(!ayumi_configure(&chip,ym,clock,sr))return 0;
 for(int i=0;i<3;i++)ayumi_set_pan(&chip,i,i*.4+.1,0);
 rate=sr;remaining=0;format=fmt;elapsed=0;return 1;
}
int rr_render(int n){
 if(n<1||n>16384)return 0;
 for(int i=0;i<n;i++){
  if(song.ended)return i;
  if(remaining<=0){song.regs[13]=255;switch(format){
 case 1:PT1_Play(song);break;case 2:PT2_Play(song);break;case 4:STP_Play(song);break;
 case 5:case 6:ASC_Play(song);break;case 7:PSC_Play(song);break;case 8:SQT_Play(song);break;case 9:STC_Play(song);break;
 case 10:{
  if(streamSkip){--streamSkip;break;}
  bool frame=false;
  for(int guard=0;guard<65536&&streamPos<streamLen;++guard){
   int cmd=module[streamPos++];if(cmd==255){frame=true;break;}if(cmd==253){song.ended=true;break;}
   if(streamPos>=streamLen){song.ended=true;break;}int v=module[streamPos++];
   if(cmd==254){if(!v){song.ended=true;break;}streamSkip=v*4-1;frame=true;break;}
   if(cmd<14)song.regs[cmd]=v;else if(cmd>15){song.ended=true;break;}
  }
  if(!frame)song.ended=true;break;
 }
 case 11:case 12:
  if(frameIndex>=dump.count){song.ended=true;break;}
  for(unsigned reg=0;reg<14;++reg)song.regs[reg]=dump.frames[dump.interleaved?reg*dump.count+frameIndex:frameIndex*dump.stride+reg];
  ++frameIndex;break;
 }if(song.ended||++elapsed>(int)tickHz*60*30){song.ended=true;return i;}
   uint8_t *r=song.regs;
   for(int j=0;j<3;j++){ayumi_set_tone(&chip,j,((r[j*2+1]&15)<<8)|r[j*2]);ayumi_set_mixer(&chip,j,(r[7]>>j)&1,(r[7]>>(j+3))&1,(r[8+j]>>4)&1);ayumi_set_volume(&chip,j,r[8+j]&15);}
   ayumi_set_noise(&chip,r[6]&31);ayumi_set_envelope(&chip,(r[12]<<8)|r[11]);if(r[13]!=255)ayumi_set_envelope_shape(&chip,r[13]&15);remaining+=rate;
  }
  remaining-=tickHz;ayumi_process(&chip);ayumi_remove_dc(&chip);output[i*2]=(float)(chip.left*.38);output[i*2+1]=(float)(chip.right*.38);
 }
 return n;
}
}
