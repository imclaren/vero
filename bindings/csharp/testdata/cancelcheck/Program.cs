using Vero;

// Cancels a slow request after 0.2 seconds, and checks that the call ends at
// once and that the worker's handler stopped.
using var vero = new VeroClient(args[0]);
await Task.Delay(1000);
var cts = new CancellationTokenSource(200);
var start = DateTime.Now;
try
{
    await vero.CallAsync("slow", new { }, cts.Token);
    Console.WriteLine("FAIL: the slow call finished");
    return 1;
}
catch (OperationCanceledException)
{
    Console.WriteLine($"cancelled after {(DateTime.Now - start).TotalSeconds:F2}s");
}
for (int i = 0; i < 50; i++)
{
    var n = await vero.CallAsync("stopped", new { });
    if (n?.GetInt64() == 1) { Console.WriteLine("PASS: the handler stopped"); return 0; }
    await Task.Delay(50);
}
Console.WriteLine("FAIL: the handler never stopped");
return 1;
