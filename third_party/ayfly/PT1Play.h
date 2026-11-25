

struct PT1_File
{
    unsigned char PT1_Delay;
    unsigned char PT1_NumberOfPositions;
    unsigned char PT1_LoopPosition;
    unsigned char PT1_SamplesPointers0[32]; //word
    unsigned char PT1_OrnamentsPointers0[32]; //word
    unsigned char PT1_PatternsPointer0, PT1_PatternsPointer1;
    signed char PT1_MusicName[30];
    unsigned char PT1_PositionList[65536 - 99];
};

#define PT1_SamplesPointers(x) (header->PT1_SamplesPointers0 [(x) * 2] | (header->PT1_SamplesPointers0 [(x) * 2 + 1] << 8))
#define PT1_OrnamentsPointers(x) (header->PT1_OrnamentsPointers0 [(x) * 2] | (header->PT1_OrnamentsPointers0 [(x) * 2 + 1] << 8))
#define PT1_PatternsPointer (header->PT1_PatternsPointer0 | (header->PT1_PatternsPointer1 << 8))

struct PT1_Channel_Parameters
{
    unsigned short Address_In_Pattern, OrnamentPointer, SamplePointer, Ton;
    unsigned char Number_Of_Notes_To_Skip, Volume, Loop_Sample_Position, Position_In_Sample, Sample_Length, Amplitude, Note;
    signed char Note_Skip_Counter;
    bool Envelope_Enabled, Enabled;
};

struct PT1_Parameters
{
    unsigned char Delay, DelayCounter, CurrentPosition;
};

struct PT1_SongInfo
{
    PT1_Parameters PT1;
    PT1_Channel_Parameters PT1_A, PT1_B, PT1_C;
};

#define PT1_A ((PT1_SongInfo *)info.data)->PT1_A
#define PT1_B ((PT1_SongInfo *)info.data)->PT1_B
#define PT1_C ((PT1_SongInfo *)info.data)->PT1_C
#define PT1 ((PT1_SongInfo *)info.data)->PT1

void PT1_Init(AYSongInfo &info)
{
    unsigned char *module = info.module;
    PT1_File *header = (PT1_File *)module;
    static PT1_SongInfo state;
    memset(&state, 0, sizeof(state));
    info.data = &state;
    memset(&PT1_A, 0, sizeof(PT1_Channel_Parameters));
    memset(&PT1_B, 0, sizeof(PT1_Channel_Parameters));
    memset(&PT1_C, 0, sizeof(PT1_Channel_Parameters));

    PT1.DelayCounter = 1;
    PT1.Delay = header->PT1_Delay;
    PT1.CurrentPosition = 0;
    PT1_A.Address_In_Pattern = ay_sys_getword(&module[PT1_PatternsPointer + header->PT1_PositionList[0] * 6]);
    PT1_B.Address_In_Pattern = ay_sys_getword(&module[PT1_PatternsPointer + header->PT1_PositionList[0] * 6 + 2]);
    PT1_C.Address_In_Pattern = ay_sys_getword(&module[PT1_PatternsPointer + header->PT1_PositionList[0] * 6 + 4]);

    PT1_A.OrnamentPointer = PT1_OrnamentsPointers(0);
    PT1_A.Envelope_Enabled = false;
    PT1_A.Position_In_Sample = 0;
    PT1_A.Enabled = false;
    PT1_A.Number_Of_Notes_To_Skip = 0;
    PT1_A.Note_Skip_Counter = 0;
    PT1_A.Volume = 15;
    PT1_A.Ton = 0;

    PT1_B.OrnamentPointer = PT1_A.OrnamentPointer;
    PT1_B.Envelope_Enabled = false;
    PT1_B.Position_In_Sample = 0;
    PT1_B.Enabled = false;
    PT1_B.Number_Of_Notes_To_Skip = 0;
    PT1_B.Note_Skip_Counter = 0;
    PT1_B.Volume = 15;
    PT1_B.Ton = 0;

    PT1_C.OrnamentPointer = PT1_A.OrnamentPointer;
    PT1_C.Envelope_Enabled = false;
    PT1_C.Position_In_Sample = 0;
    PT1_C.Enabled = false;
    PT1_C.Number_Of_Notes_To_Skip = 0;
    PT1_C.Note_Skip_Counter = 0;
    PT1_C.Volume = 15;
    PT1_C.Ton = 0;

    ay_resetay(&info, 0);
}

void PT1_PatternInterpreter(AYSongInfo &info, PT1_Channel_Parameters &chan)
{
    unsigned char *module = info.module;
    PT1_File *header = (PT1_File *)module;
    bool quit = false;
    unsigned guard=0;
    do
    {
        unsigned char val = module[checked_offset(chan.Address_In_Pattern)];
        if(val <= 0x5f)
        {
            chan.Note = val;
            chan.Enabled = true;
            chan.Position_In_Sample = 0;
            quit = true;
        }
        else if(val >= 0x60 && val <= 0x6f)
        {
            chan.SamplePointer = PT1_SamplesPointers(val - 0x60);
            chan.Sample_Length = module[checked_offset(chan.SamplePointer)];
            chan.SamplePointer++;
            chan.Loop_Sample_Position = module[checked_offset(chan.SamplePointer)];
            chan.SamplePointer++;
        }
        else if(val >= 0x70 && val <= 0x7f)
            chan.OrnamentPointer = PT1_OrnamentsPointers(val - 0x70);
        else if(val == 0x80)
        {
            chan.Enabled = false;
            quit = true;
        }
        else if(val == 0x81)
            chan.Envelope_Enabled = false;
        else if(val >= 0x82 && val <= 0x8f)
        {
            chan.Envelope_Enabled = true;
            ay_writeay(&info, AY_ENV_SHAPE, val - 0x81);
            chan.Address_In_Pattern++;
            ay_writeay(&info, AY_ENV_FINE, module[checked_offset(chan.Address_In_Pattern)]);
            chan.Address_In_Pattern++;
            ay_writeay(&info, AY_ENV_COARSE, module[checked_offset(chan.Address_In_Pattern)]);
        }
        else if(val == 0x90)
            quit = true;
        else if(val >= 0x91 && val <= 0xa0)
            PT1.Delay = val - 0x91;
        else if(val >= 0xa1 && val <= 0xb0)
            chan.Volume = val - 0xa1;
        else
            chan.Number_Of_Notes_To_Skip = val - 0xb1;
        chan.Address_In_Pattern++;
    }
    while(!quit && ++guard<2048);
    chan.Note_Skip_Counter = chan.Number_Of_Notes_To_Skip;
}

void PT1_GetRegisters(AYSongInfo &info, PT1_Channel_Parameters &chan, unsigned char &TempMixer)
{
    unsigned char *module = info.module;
    unsigned char j, b;
    if(chan.Enabled)
    {
        j = chan.Note + module[checked_offset(chan.OrnamentPointer + chan.Position_In_Sample)];
        if(j > 95)
            j = 95;
        b = module[checked_offset(chan.SamplePointer + chan.Position_In_Sample * 3)];
        chan.Ton = ((((unsigned short)(b)) << 4) & 0xf00) + module[checked_offset(chan.SamplePointer + chan.Position_In_Sample * 3 + 2)];
        chan.Amplitude = (chan.Volume * 17 + (chan.Volume > 7 ? 1 : 0)) * (b & 15) / 256;
        b = module[checked_offset(chan.SamplePointer + chan.Position_In_Sample * 3 + 1)];
        if((b & 32) == 0)
            chan.Ton = -chan.Ton;
        chan.Ton = (chan.Ton + PT2_Table[note_index(j,sizeof(PT2_Table)/sizeof(PT2_Table[0]))] + (j == 46 ? 1 : 0)) & 0xfff;
        if(chan.Envelope_Enabled)
            chan.Amplitude |= 16;
        if((signed char)b < 0)
            TempMixer = TempMixer | 64;
        else
            ay_writeay(&info, AY_NOISE_PERIOD, b & 31);
        if((b & 64) != 0)
            TempMixer = TempMixer | 8;
        chan.Position_In_Sample++;
        if(chan.Position_In_Sample == chan.Sample_Length)
            chan.Position_In_Sample = chan.Loop_Sample_Position;
    }
    else
        chan.Amplitude = 0;
    TempMixer = TempMixer >> 1;

}

void PT1_Play(AYSongInfo &info)
{
    unsigned char *module = info.module;
    PT1_File *header = (PT1_File *)module;
    unsigned char TempMixer;
    PT1.DelayCounter--;
    if(PT1.DelayCounter == 0)
    {
        PT1_A.Note_Skip_Counter--;
        if(PT1_A.Note_Skip_Counter < 0)
        {
            if(module[checked_offset(PT1_A.Address_In_Pattern)] == 255)
            {
                PT1.CurrentPosition++;
                if(PT1.CurrentPosition == header->PT1_NumberOfPositions)
                    { info.ended=true; return; }
                PT1_A.Address_In_Pattern = ay_sys_getword(&module[PT1_PatternsPointer + header->PT1_PositionList[PT1.CurrentPosition] * 6]);
                PT1_B.Address_In_Pattern = ay_sys_getword(&module[PT1_PatternsPointer + header->PT1_PositionList[PT1.CurrentPosition] * 6 + 2]);
                PT1_C.Address_In_Pattern = ay_sys_getword(&module[PT1_PatternsPointer + header->PT1_PositionList[PT1.CurrentPosition] * 6 + 4]);
            }
            PT1_PatternInterpreter(info, PT1_A);
        }
        PT1_B.Note_Skip_Counter--;
        if(PT1_B.Note_Skip_Counter < 0)
            PT1_PatternInterpreter(info, PT1_B);
        PT1_C.Note_Skip_Counter--;
        if(PT1_C.Note_Skip_Counter < 0)
            PT1_PatternInterpreter(info, PT1_C);
        PT1.DelayCounter = PT1.Delay;
    }
    TempMixer = 0;
    PT1_GetRegisters(info, PT1_A, TempMixer);
    PT1_GetRegisters(info, PT1_B, TempMixer);
    PT1_GetRegisters(info, PT1_C, TempMixer);

    ay_writeay(&info, AY_MIXER, TempMixer);

    ay_writeay(&info, AY_CHNL_A_FINE, PT1_A.Ton & 0xff);
    ay_writeay(&info, AY_CHNL_A_COARSE, (PT1_A.Ton >> 8) & 0xf);
    ay_writeay(&info, AY_CHNL_B_FINE, PT1_B.Ton & 0xff);
    ay_writeay(&info, AY_CHNL_B_COARSE, (PT1_B.Ton >> 8) & 0xf);
    ay_writeay(&info, AY_CHNL_C_FINE, PT1_C.Ton & 0xff);
    ay_writeay(&info, AY_CHNL_C_COARSE, (PT1_C.Ton >> 8) & 0xf);
    ay_writeay(&info, AY_CHNL_A_VOL, PT1_A.Amplitude);
    ay_writeay(&info, AY_CHNL_B_VOL, PT1_B.Amplitude);
    ay_writeay(&info, AY_CHNL_C_VOL, PT1_C.Amplitude);
}





bool PT1_Detect(unsigned char *module, unsigned long length)
{
    int j, j1, j2;
    PT1_File *header = (PT1_File *)module;
    if(length < 0x66)
        return false;
    if(PT1_PatternsPointer >= length)
        return false;

    j = 0;
    j1 = 65535;
    for(j2 = 0; j2 <= 15; j2++)
    {
        if(j < PT1_SamplesPointers(j2))
            j = PT1_SamplesPointers(j2);
        if((PT1_OrnamentsPointers(j2) != 0) && (j1 > PT1_OrnamentsPointers(j2)))
            j1 = PT1_OrnamentsPointers(j2);
    }

    if(j1 < 0x67)
        return false;
    if(j < 0x67)
        return false;
    if(j > 65534)
        return false;
    if(j > length)
        return false;
    if((j + module[checked_offset(j)] * 3 + 2) != j1)
        return false;

    j = 0;
    for(j2 = 0; j2 <= 15; j2++)
        if(j < PT1_OrnamentsPointers(j2))
            j = PT1_OrnamentsPointers(j2);
    if(j < 0x67)
        return false;
    int F_Length = j + 64;
    if(F_Length > 65536)
        return false;
    if(F_Length > length + 1)
        return false;

    j = 0x63;
    while((j <= PT1_PatternsPointer) && (module[checked_offset(j)] != 255))
        j++;
    if(j + 1 != PT1_PatternsPointer)
        return false;

    header->PT1_NumberOfPositions = j - 0x63;
    return true;
}
