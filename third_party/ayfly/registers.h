// Bounded register-dump decoder: PSG streams, VTX and YM (including LH5).
// LH5 uses canonical Huffman codes and a 8192-byte sliding dictionary.
struct Bits {
 const uint8_t *p; size_t size,pos=0; bool ok=true;
 unsigned get(unsigned n){if(n>16||pos+n>size*8){ok=false;return 0;}unsigned v=0;while(n--){v=(v<<1)|((p[pos/8]>>(7-pos%8))&1);++pos;}return v;}
};
struct Huffman {
 unsigned count[17],first[17],base[17],symbols[510];int single;
 bool build(const uint8_t *len,unsigned n,int fixed=-1){
  memset(count,0,sizeof(count));single=fixed;if(fixed>=0)return (unsigned)fixed<n;
  for(unsigned i=0;i<n;i++){if(len[i]>16)return false;if(len[i])++count[len[i]];}
  unsigned code=0,off=0;
  for(unsigned k=1;k<=16;k++){code=(code+count[k-1])*2;first[k]=code;base[k]=off;off+=count[k];if(code+count[k]>(1u<<k))return false;}
  if(!off)return false;
  unsigned next[17];memcpy(next,base,sizeof(next));for(unsigned i=0;i<n;i++)if(len[i])symbols[next[len[i]]++]=i;
  return true;
 }
 int read(Bits &b){if(single>=0)return single;unsigned c=0;for(unsigned k=1;k<=16&&b.ok;k++){c=(c<<1)|b.get(1);if(c>=first[k]&&c-first[k]<count[k])return symbols[base[k]+c-first[k]];}b.ok=false;return 0;}
};
static bool readPT(Bits &b,Huffman &h,unsigned n,unsigned width,int special){
 uint8_t lens[19]={};unsigned count=b.get(width);if(!count){int s=b.get(width);return b.ok&&h.build(lens,n,s);}
 if(count>n)return false;
 for(unsigned i=0;i<count;){unsigned c=b.get(3);if(c==7){while(b.ok&&b.get(1)){if(++c>16)return false;}}if(!b.ok)return false;lens[i++]=c;if((int)i==special){unsigned z=b.get(2);if(i+z>n)return false;i+=z;}}
 return b.ok&&h.build(lens,n);
}
static bool readCLens(Bits &b,Huffman &h,Huffman &pt){
 uint8_t lens[510]={};unsigned n=b.get(9);if(!n){int s=b.get(9);return b.ok&&h.build(lens,510,s);}if(n>510)return false;
 for(unsigned i=0;i<n&&b.ok;){int c=pt.read(b);if(c<=2){unsigned z=c==0?1:c==1?b.get(4)+3:b.get(9)+20;if(i+z>n)return false;i+=z;}else{if(c>18)return false;lens[i++]=c-2;}}
 return b.ok&&h.build(lens,510);
}
static bool lh5(const uint8_t *src,size_t size,uint8_t *out,size_t len){
 Bits b{src,size};Huffman pt{},c{},p{};unsigned block=0;size_t off=0;
 while(off<len&&b.ok){if(!block){block=b.get(16);if(!block||!readPT(b,pt,19,5,3)||!readCLens(b,c,pt)||!readPT(b,p,14,4,-1))return false;}
  --block;int v=c.read(b);if(!b.ok)return false;
  if(v<256)out[off++]=(uint8_t)v;
  else{unsigned n=v-253,k=p.read(b);if(!b.ok||k>13)return false;unsigned dist=k?(1u<<(k-1))+b.get(k-1)+1:1;if(!b.ok||dist>8192||n>len-off)return false;
   while(n--){out[off]=dist>off?' ':out[off-dist];++off;}
  }
 }
 return b.ok&&off==len;
}
static unsigned le32(const uint8_t *p){return (unsigned)p[0]|((unsigned)p[1]<<8)|((unsigned)p[2]<<16)|((unsigned)p[3]<<24);}
static unsigned be32(const uint8_t *p){return (unsigned)p[3]|((unsigned)p[2]<<8)|((unsigned)p[1]<<16)|((unsigned)p[0]<<24);}
static unsigned be16(const uint8_t *p){return (p[0]<<8)|p[1];}
static bool match(const uint8_t *p,const char *s,unsigned n){for(unsigned i=0;i<n;i++)if(p[i]!=(uint8_t)s[i])return false;return true;}
static bool stringsEnd(const uint8_t *p,size_t len,size_t &off,unsigned count){while(count--){while(off<len&&p[off])++off;if(off>=len)return false;++off;}return true;}
struct Dump {const uint8_t *frames;unsigned count,stride,hz,clock,ym;bool interleaved;};
static bool decodeVTX(const uint8_t *p,size_t len,uint8_t *buf,size_t cap,Dump &d){
 if(len<17)return false;bool newer=match(p,"ay",2)||match(p,"ym",2),older=match(p,"AY",2)||match(p,"YM",2);
 if(!newer&&!older)return false;
 unsigned clock=le32(p+5),hz=p[9],size=le32(p+(newer?12:10));size_t off=newer?16:14;
 if(!size||size>cap||size%14||!stringsEnd(p,len,off,newer?5:2)||off>=len||!lh5(p+off,len-off,buf,size))return false;
 d={buf,size/14,14,hz,clock,(unsigned)(p[0]=='y'||p[0]=='Y'),true};return true;
}
static bool decodeYM(const uint8_t *p,size_t len,uint8_t *buf,size_t cap,Dump &d){
 if(len<4)return false;
 if(len>=24&&match(p+2,"-lh5-",5)){
  unsigned packed=le32(p+7),size=le32(p+11);size_t off=p[0]+2;
  // Level 0 LHA, one file. Validate declared packed and expanded lengths.
  if(p[20]!=0||off<24||off>len||!size||size>cap||packed>len-off||!lh5(p+off,packed,buf,size))return false;p=buf;len=size;
 }
 if(match(p,"YM3!",4)||match(p,"YM3b",4)){
  unsigned tail=match(p,"YM3b",4)?4:0;if(len<=4+tail||(len-4-tail)%14)return false;
  d={p+4,(unsigned)(len-4-tail)/14,14,50,2000000,1,true};return true;
 }
 if(len<34||(!match(p,"YM5!",4)&&!match(p,"YM6!",4))||!match(p+4,"LeOnArD!",8))return false;
 unsigned frames=be32(p+12),flags=be32(p+16),drums=be16(p+20),clock=be32(p+22),hz=be16(p+26);size_t off=34+be16(p+32);
 // Sampled drum / extended effects are not emulated in this register-only player.
 if(drums||off>len||!stringsEnd(p,len,off,3)||!frames||frames>(len-off)/16)return false;
 d={p+off,frames,16,hz,clock,1,(flags&1)!=0};return true;
}
