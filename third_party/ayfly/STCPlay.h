

struct STC_File
{
    unsigned char ST_Delay;
    unsigned char ST_PositionsPointer0, ST_PositionsPointer1;
    unsigned char ST_OrnamentsPointer0, ST_OrnamentsPointer1;
    unsigned char ST_PatternsPointer0, ST_PatternsPointer1;
    signed char ST_Name[18];
    unsigned char ST_Size0, ST_Size1;
};

struct STC_Channel_Parameters
{
    unsigned short Address_In_Pattern, SamplePointer, OrnamentPointer, Ton;
    unsigned char Amplitude, Note, Position_In_Sample, Number_Of_Notes_To_Skip;
    signed char Sample_Tik_Counter, Note_Skip_Counter;
    bool Envelope_Enabled;
};

struct STC_Parameters
{
    unsigned char DelayCounter, Transposition, CurrentPosition;
};

struct STC_SongInfo
{
    STC_Parameters STC;
    STC_Channel_Parameters STC_A, STC_B, STC_C;
};

#define STC_A ((STC_SongInfo *)info.data)->STC_A
#define STC_B ((STC_SongInfo *)info.data)->STC_B
#define STC_C ((STC_SongInfo *)info.data)->STC_C
#define STC ((STC_SongInfo *)info.data)->STC

#define ST_PositionsPointer (header->ST_PositionsPointer0 | (header->ST_PositionsPointer1 << 8))
#define ST_OrnamentsPointer (header->ST_OrnamentsPointer0 | (header->ST_OrnamentsPointer1 << 8))
#define ST_PatternsPointer (header->ST_PatternsPointer0 | (header->ST_PatternsPointer1 << 8))
#define ST_Size (header->ST_Size0 | (header->ST_Size1 << 8))
#define ST_Delay (header->ST_Delay)

void STC_Init(AYSongInfo &info)
{
    unsigned char *module = info.module;
    STC_File *header = (STC_File *)module;
    static STC_SongInfo state;
    memset(&state, 0, sizeof(state));
    info.data = &state;
    memset(&STC_A, 0, sizeof(STC_Channel_Parameters));
    memset(&STC_B, 0, sizeof(STC_Channel_Parameters));
    memset(&STC_C, 0, sizeof(STC_Channel_Parameters));

    STC.CurrentPosition = 0;
    STC.Transposition = module[checked_offset(ST_PositionsPointer + 2)];
    STC.DelayCounter = 1;

    unsigned long i = 0;
    while(i<256 && module[checked_offset(ST_PatternsPointer + 7 * i)] != module[checked_offset(ST_PositionsPointer + 1)]) i++;
    if(i==256){info.ended=true;return;}
    STC_A.Address_In_Pattern = ay_sys_getword(&module[checked_offset(ST_PatternsPointer + 7 * i + 1)]);
    STC_B.Address_In_Pattern = ay_sys_getword(&module[checked_offset(ST_PatternsPointer + 7 * i + 3)]);
    STC_C.Address_In_Pattern = ay_sys_getword(&module[checked_offset(ST_PatternsPointer + 7 * i + 5)]);

    STC_A.Note_Skip_Counter = 0;
    STC_A.Envelope_Enabled = false;
    STC_A.Number_Of_Notes_To_Skip = 0;
    STC_A.Sample_Tik_Counter = -1;
    STC_A.Position_In_Sample = 0;
    STC_A.OrnamentPointer = ST_OrnamentsPointer + 1;
    STC_A.Ton = 0;

    STC_B.Note_Skip_Counter = 0;
    STC_B.Envelope_Enabled = false;
    STC_B.Number_Of_Notes_To_Skip = 0;
    STC_B.Sample_Tik_Counter = -1;
    STC_B.Position_In_Sample = 0;
    STC_B.OrnamentPointer = ST_OrnamentsPointer + 1;
    STC_B.Ton = 0;

    STC_C.Note_Skip_Counter = 0;
    STC_C.Envelope_Enabled = false;
    STC_C.Number_Of_Notes_To_Skip = 0;
    STC_C.Sample_Tik_Counter = -1;
    STC_C.Position_In_Sample = 0;
    STC_C.OrnamentPointer = ST_OrnamentsPointer + 1;
    STC_C.Ton = 0;

    ay_resetay(&info, 0);
}



void STC_PatternInterpreter(AYSongInfo &info, STC_Channel_Parameters &chan)
{
    unsigned short k;
    unsigned char *module = info.module;
    STC_File *header = (STC_File *)module;
    for(unsigned guard=0; guard<2048; ++guard)
    {
        unsigned char val = module[checked_offset(chan.Address_In_Pattern)];
        if(val <= 0x5f)
        {
            chan.Note = val;
            chan.Sample_Tik_Counter = 32;
            chan.Position_In_Sample = 0;
            chan.Address_In_Pattern++;
            break;
        }
        else if(val >= 0x60 && val <= 0x6f)
        {
            k = 0;
            while(k<16 && module[checked_offset(0x1b + 0x63 * k)] != (val - 0x60)) k++;
            if(k==16){info.ended=true;return;}
            chan.SamplePointer = 0x1c + 0x63 * k;
        }
        else if(val >= 0x70 && val <= 0x7f)
        {
            k = 0;
            while(k<16 && module[checked_offset(ST_OrnamentsPointer + 0x21 * k)] != (val - 0x70)) k++;
            if(k==16){info.ended=true;return;}
            chan.OrnamentPointer = ST_OrnamentsPointer + 0x21 * k + 1;
            chan.Envelope_Enabled = false;
        }
        else if(val == 0x80)
        {
            chan.Sample_Tik_Counter = -1;
            chan.Address_In_Pattern++;
            break;
        }
        else if(val == 0x81)
        {
            chan.Address_In_Pattern++;
            break;
        }
        else if(val == 0x82)
        {
            k = 0;
            while(k<16 && module[checked_offset(ST_OrnamentsPointer + 0x21 * k)] != 0) k++;
            if(k==16){info.ended=true;return;}
            chan.OrnamentPointer = ST_OrnamentsPointer + 0x21 * k + 1;
            chan.Envelope_Enabled = false;
        }
        else if(val >= 0x83 && val <= 0x8e)
        {
            ay_writeay(&info, AY_ENV_SHAPE, val - 0x80);
            chan.Address_In_Pattern++;
            ay_writeay(&info, AY_ENV_FINE, module[checked_offset(chan.Address_In_Pattern)]);
            chan.Envelope_Enabled = true;
            k = 0;
            while(k<16 && module[checked_offset(ST_OrnamentsPointer + 0x21 * k)] != 0) k++;
            if(k==16){info.ended=true;return;}
            chan.OrnamentPointer = ST_OrnamentsPointer + 0x21 * k + 1;
        }
        else
            chan.Number_Of_Notes_To_Skip = val - 0xa1;
        chan.Address_In_Pattern++;
    }
    chan.Note_Skip_Counter = chan.Number_Of_Notes_To_Skip;
}

void STC_GetRegisters(AYSongInfo &info, STC_Channel_Parameters &chan, unsigned char &TempMixer)
{
    unsigned short i;
    unsigned char j;
    unsigned char *module = info.module;
    STC_File *header = (STC_File *)module;
    if(chan.Sample_Tik_Counter >= 0)
    {
        chan.Sample_Tik_Counter--;
        chan.Position_In_Sample = (chan.Position_In_Sample + 1) & 0x1f;
        if(chan.Sample_Tik_Counter == 0)
        {
            if(module[checked_offset(chan.SamplePointer + 0x60)] != 0)
            {
                chan.Position_In_Sample = module[checked_offset(chan.SamplePointer + 0x60)] & 0x1f;
                chan.Sample_Tik_Counter = module[checked_offset(chan.SamplePointer + 0x61)] + 1;
            }
            else
                chan.Sample_Tik_Counter = -1;
        }
    }
    if(chan.Sample_Tik_Counter >= 0)
    {
        i = ((chan.Position_In_Sample - 1) & 0x1f) * 3 + chan.SamplePointer;
        if((module[checked_offset(i + 1)] & 0x80) != 0)
            TempMixer = TempMixer | 64;
        else
            ay_writeay(&info, AY_NOISE_PERIOD, module[checked_offset(i + 1)] & 0x1f);
        if((module[checked_offset(i + 1)] & 0x40) != 0)
            TempMixer = TempMixer | 8;
        chan.Amplitude = module[checked_offset(i)] & 15;
        j = chan.Note + module[checked_offset(chan.OrnamentPointer + ((chan.Position_In_Sample - 1) & 0x1f))] + STC.Transposition;
        if(j > 95)
            j = 95;
        if((module[checked_offset(i + 1)] & 0x20) != 0)
            chan.Ton = (ST_Table[note_index(j,sizeof(ST_Table)/sizeof(ST_Table[0]))] + module[checked_offset(i + 2)] + (((unsigned short)(module[checked_offset(i)] & 0xf0)) << 4)) & 0xFFF;
        else
            chan.Ton = (ST_Table[note_index(j,sizeof(ST_Table)/sizeof(ST_Table[0]))] - module[checked_offset(i + 2)] - (((unsigned short)(module[checked_offset(i)] & 0xf0)) << 4)) & 0xFFF;
        if(chan.Envelope_Enabled)
            chan.Amplitude = chan.Amplitude | 16;
    }
    else
        chan.Amplitude = 0;

    TempMixer = TempMixer >> 1;
}

void STC_Play(AYSongInfo &info)
{
    unsigned char TempMixer;
    unsigned short i;
    unsigned char *module = info.module;
    STC_File *header = (STC_File *)module;

    STC.DelayCounter--;
    if(STC.DelayCounter == 0)
    {
        STC.DelayCounter = ST_Delay;
        STC_A.Note_Skip_Counter--;
        if(STC_A.Note_Skip_Counter < 0)
        {
            if(module[checked_offset(STC_A.Address_In_Pattern)] == 255)
            {
                if(STC.CurrentPosition == module[checked_offset(ST_PositionsPointer)])
                    { info.ended=true; return; }
                else
                    STC.CurrentPosition++;
                STC.Transposition = module[checked_offset(ST_PositionsPointer + 2 + STC.CurrentPosition * 2)];
                i = 0;
                while(i<256 && module[checked_offset(ST_PatternsPointer + 7 * i)] != module[checked_offset(ST_PositionsPointer + 1 + STC.CurrentPosition * 2)]) i++;
    if(i==256){info.ended=true;return;}
                STC_A.Address_In_Pattern = ay_sys_getword(&module[checked_offset(ST_PatternsPointer + 7 * i + 1)]);
                STC_B.Address_In_Pattern = ay_sys_getword(&module[checked_offset(ST_PatternsPointer + 7 * i + 3)]);
                STC_C.Address_In_Pattern = ay_sys_getword(&module[checked_offset(ST_PatternsPointer + 7 * i + 5)]);
            }
            STC_PatternInterpreter(info, STC_A);
        }

        STC_B.Note_Skip_Counter--;
        if(STC_B.Note_Skip_Counter < 0)
            STC_PatternInterpreter(info, STC_B);
        STC_C.Note_Skip_Counter--;
        if(STC_C.Note_Skip_Counter < 0)
            STC_PatternInterpreter(info, STC_C);
    }

    TempMixer = 0;
    STC_GetRegisters(info, STC_A, TempMixer);
    STC_GetRegisters(info, STC_B, TempMixer);
    STC_GetRegisters(info, STC_C, TempMixer);

    ay_writeay(&info, AY_MIXER, TempMixer);

    ay_writeay(&info, AY_CHNL_A_FINE, STC_A.Ton & 0xff);
    ay_writeay(&info, AY_CHNL_A_COARSE, (STC_A.Ton >> 8) & 0xf);
    ay_writeay(&info, AY_CHNL_B_FINE, STC_B.Ton & 0xff);
    ay_writeay(&info, AY_CHNL_B_COARSE, (STC_B.Ton >> 8) & 0xf);
    ay_writeay(&info, AY_CHNL_C_FINE, STC_C.Ton & 0xff);
    ay_writeay(&info, AY_CHNL_C_COARSE, (STC_C.Ton >> 8) & 0xf);
    ay_writeay(&info, AY_CHNL_A_VOL, STC_A.Amplitude);
    ay_writeay(&info, AY_CHNL_B_VOL, STC_B.Amplitude);
    ay_writeay(&info, AY_CHNL_C_VOL, STC_C.Amplitude);
}



bool STC_Detect(unsigned char *module, unsigned long length)
{
    STC_File *header = (STC_File *)module;
    int j, j1, j2;
    if(length < 6)
        return false;
    if(ST_PositionsPointer> length)
        return false;
    if((int)(ST_PatternsPointer- ST_OrnamentsPointer) <= 0)
    return false;
    if(((ST_PatternsPointer - ST_OrnamentsPointer) % 0x21) != 0)
    return false;
    if((int)(ST_PositionsPointer - ST_OrnamentsPointer) >= 0)
    return false;
    if((module[checked_offset(ST_PositionsPointer)] * 2 + 3 + ST_PositionsPointer - ST_OrnamentsPointer) != 0)
    return false;

    j = ST_OrnamentsPointer + 0x21;
    if(j> 65535)
    return false;
    if(j> length)
    return false;
    do
    {
        j--;
        if(module[checked_offset(j)] != 0)
        return false;;
    }
    while(j != ST_OrnamentsPointer);

    j = ST_PatternsPointer;
    if(j> length)
    return false;
    j1 = 0;
    j2 = 0;
    while((j + 6 <= length) && (j + 6 < 65536) && (module[checked_offset(j)] != 255))
    {
        j++;
        j2 = ay_sys_getword(&module[checked_offset(j)]);
        if(j1 < j2)
        j1 = j2;
        j += 2;
        j2 = ay_sys_getword(&module[checked_offset(j)]);
        if(j1 < j2)
        j1 = j2;
        j += 2;
        j2 = ay_sys_getword(&module[checked_offset(j)]);
        if(j1 < j2)
        j1 = j2;
        j += 2;
    }
    if(module[checked_offset(j)] != 255)
    return false;
    if(j1> length)
    return false;
    if(j1<=0 || module[checked_offset(j1 - 1)] != 255)
    return false;
    do
    {
        if((module[checked_offset(j1)] >= 0x83) && (module[checked_offset(j1)] <= 0x8e))
        j1++;
        j1++;
    }
    while((j1 <= 65535) && (j1 < length) && (module[checked_offset(j1)] != 255));
    if(j1> 65535)
    return false;
    if(j1> length)
    return false;

    return true;
}
