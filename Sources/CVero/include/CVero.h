// Declarations for the archive built from ./cshim.
//
//   go build -buildmode=c-archive -o libvero.a ./cshim
//
// This header is written by hand rather than taken from the generated
// libvero.h, because the generated one carries the whole cgo preamble and
// SwiftPM only needs these seven symbols.
#ifndef CVERO_H
#define CVERO_H

extern char* VeroStart(char* workerPath, char* argsJSON);
extern char* VeroRequest(char* requestJSON);
extern char* VeroCall(char* name, char* requestJSON);
extern char* VeroCallTagged(unsigned long long tag, char* name, char* requestJSON);
extern void VeroCancel(unsigned long long tag);
extern char* VeroLatest(void);
extern char* VeroWaitForEvent(void);
extern char* VeroState(void);
extern char* VeroRestarts(void);
extern void  VeroStop(void);
extern void  VeroFree(char* s);

#endif
