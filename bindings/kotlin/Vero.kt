// Written by vero. Do not edit it: vero upgrade replaces it with each version
// of vero, and stops if it has been changed.
//
// Drive a Go worker from a Kotlin frontend - Android, or anything else on the
// JVM.
//
// The worker supervises itself.  Run with VERO_HOST set it becomes a host: it
// launches a second copy of itself to do the work, restarts that copy if it
// dies, and speaks JSON on its own standard input and output.  So this file
// starts one process and writes lines to it; the supervision, the backoff,
// the single-worker lock and the cleanup are all Go, on the other side of the
// pipe.
//
//     val vero = Vero(File(applicationInfo.nativeLibraryDir, "libworker.so").path)
//     vero.events { status -> runOnUiThread { draw(status) } }
//
// There is no shared library to build, ship or match to an architecture - the
// worker is an ordinary executable, which is why this works anywhere Go
// produces one.
package com.imclaren.vero

import org.json.JSONObject
import java.io.BufferedReader
import java.io.File
import java.io.IOException
import java.util.concurrent.ArrayBlockingQueue
import java.util.concurrent.BlockingQueue
import java.util.concurrent.ConcurrentHashMap
import java.util.concurrent.atomic.AtomicInteger

/** Something went wrong talking to the worker. */
open class VeroException(message: String) : Exception(message)

/**
 * The worker received the request and refused it.  It is still running, so
 * this is a problem with the request: show the message and carry on.
 */
class Refused(message: String) : VeroException(message)

/**
 * Another process is already running a worker for this application.  Offer to
 * switch to it: retrying will not help, and nothing is broken.
 */
class AlreadyRunning(message: String) : VeroException(message)

/**
 * The worker is starting, restarting after a crash, or stopped.  Nothing the
 * caller did was wrong: wait, and say so in the frontend.
 */
class NotRunning(message: String) : VeroException(message)

/**
 * The request was cancelled, through a [Cancel], before its reply came.
 * Nobody is waiting for it any longer, so do not show it as a failure.
 */
class Cancelled(message: String) : VeroException(message)

/**
 * Cancels the requests given it when [cancel] is called, from any thread:
 * each raises [Cancelled] at once, and the worker is told, so that its
 * handler can stop. A worker older than cancelling finishes the work, and
 * its reply is dropped.
 *
 *     val token = Cancel()
 *     thread { vero.call("thumbnail", request, token) }
 *     token.cancel()
 */
class Cancel {
    private val hooks = mutableListOf<() -> Unit>()

    @Volatile
    var cancelled = false
        private set

    fun cancel() {
        val run = synchronized(this) {
            cancelled = true
            hooks.toList().also { hooks.clear() }
        }
        run.forEach { it() }
    }

    internal fun watch(hook: () -> Unit): Boolean = synchronized(this) {
        if (cancelled) false else { hooks.add(hook); true }
    }

    internal fun unwatch(hook: () -> Unit) = synchronized(this) { hooks.remove(hook) }
}

private fun errorFor(code: String?, message: String): VeroException = when (code) {
    "cancelled" -> Cancelled(message)
    "refused" -> Refused(message)
    "already_running" -> AlreadyRunning(message)
    "not_running" -> NotRunning(message)
    else -> VeroException(message)
}

/** Runs a Go worker and talks to it. */
class Vero(workerPath: String, arguments: List<String> = emptyList()) {

    private val process: Process = ProcessBuilder(listOf(workerPath) + arguments)
        .apply { environment()["VERO_HOST"] = "1" }
        .start()

    private val nextId = AtomicInteger(0)
    private val pending = ConcurrentHashMap<Int, BlockingQueue<JSONObject>>()
    private val subscribers = mutableListOf<(JSONObject?) -> Unit>()

    @Volatile private var latest: JSONObject? = null
    @Volatile private var workerState: String = "starting"
    @Volatile private var restartCount: Int = 0
    @Volatile private var stopped: Boolean = false

    // A daemon thread, so an application that forgets to stop() still exits.
    // The worker goes with it either way: its standard input closes when this
    // process does.
    private val reader = Thread({ read() }, "vero-reader").apply {
        isDaemon = true
        start()
    }

    /** Sorts what the host says into replies, events and state changes. */
    private fun read() {
        process.inputStream.bufferedReader().use { stream: BufferedReader ->
            while (true) {
                val line = stream.readLine() ?: break
                val envelope = try {
                    JSONObject(line)
                } catch (notOurs: Exception) {
                    continue // a worker's own prints go to standard error
                }
                when (envelope.optString("t")) {
                    "reply" -> pending.remove(envelope.optInt("id"))?.put(envelope)
                    "event" -> {
                        val payload = envelope.optJSONObject("p")
                        latest = payload
                        synchronized(subscribers) { subscribers.toList() }
                            .forEach { it(payload) }
                    }
                    "state" -> envelope.optJSONObject("p")?.let {
                        workerState = it.optString("state", workerState)
                        restartCount = it.optInt("restarts", restartCount)
                    }
                }
            }
        }

        // The host has gone: wake everything waiting on it rather than leaving
        // a frontend blocked for good.
        stopped = true
        val gone = JSONObject()
            .put("e", "the worker host has stopped")
            .put("c", "not_running")
        pending.values.forEach { it.offer(gone) }
        pending.clear()
        synchronized(subscribers) { subscribers.toList() }.forEach { it(null) }
    }

    /** Writes one request and waits for its reply. */
    private fun ask(kind: String, name: String, payload: Any?, cancel: Cancel? = null): JSONObject? {
        if (stopped) throw NotRunning("the worker host has stopped")

        val id = nextId.incrementAndGet()
        val slot: BlockingQueue<JSONObject> = ArrayBlockingQueue(1)
        pending[id] = slot

        val request = JSONObject().put("id", id)
        if (kind.isNotEmpty()) request.put("t", kind)
        if (name.isNotEmpty()) request.put("n", name)
        if (payload != null) request.put("p", payload)

        try {
            synchronized(process) {
                process.outputStream.write((request.toString() + "\n").toByteArray())
                process.outputStream.flush()
            }
        } catch (broken: IOException) {
            pending.remove(id)
            throw NotRunning("the worker host has stopped")
        }

        val hook = {
            if (pending.remove(id) != null) {
                try {
                    synchronized(process) {
                        process.outputStream.write((JSONObject().put("t", "cancel").put("id", id).toString() + "\n").toByteArray())
                        process.outputStream.flush()
                    }
                } catch (broken: IOException) {
                    // It has gone, and the request with it.
                }
                slot.offer(JSONObject().put("e", "the request was cancelled").put("c", "cancelled"))
            }
        }
        if (cancel != null && !cancel.watch(hook)) hook()
        val envelope = try {
            slot.take()
        } finally {
            cancel?.unwatch(hook)
        }
        val message = envelope.optString("e")
        if (message.isNotEmpty()) {
            throw errorFor(envelope.optString("c").ifEmpty { null }, message)
        }
        return envelope.optJSONObject("p")
    }

    /**
     * Sends a request to one named handler, matching vero.Update on the
     * worker, and waits for the reply.
     *
     * There is no timeout: a worker may hold a request for as long as the work
     * takes, so call this off whatever thread draws the frontend. [cancel],
     * when given, cancels it.
     */
    fun call(name: String, request: Any? = null, cancel: Cancel? = null): JSONObject? = ask("", name, request, cancel)

    /** Sends a request the worker routes on its own contents. */
    fun send(request: Any, cancel: Cancel? = null): JSONObject? = ask("", "", request, cancel)

    /**
     * The most recent event, without waiting for the next one.  Use it to draw
     * a window that has just opened; [events] keeps it up to date afterwards.
     */
    fun latest(): JSONObject? = latest ?: ask("ctl", "latest", null)

    /** What the worker is doing: starting, running, restarting or stopped. */
    fun state(): String = workerState

    /** How many times the worker has been relaunched without one lasting. */
    fun restarts(): Int = restartCount

    /**
     * Calls [onStatus] with every event the worker emits, on the reader
     * thread, and with null once the worker has gone.  An Android frontend
     * hands its own work back to the main thread from in here.
     */
    fun events(onStatus: (JSONObject?) -> Unit) {
        synchronized(subscribers) { subscribers.add(onStatus) }
        latest?.let(onStatus)
    }

    /**
     * Stops the worker.  Not required - the worker's standard input closes
     * when this process exits, and it goes with it - but it stops the work a
     * moment sooner.
     */
    fun stop() {
        if (!stopped) {
            try {
                ask("ctl", "stop", null)
            } catch (alreadyGone: VeroException) {
                // It has gone, which is what was asked for.
            }
        }
        process.destroy()
    }

    companion object {
        /**
         * The worker inside an Android application.
         *
         * Android only runs executables from the application's own native
         * library directory, and only extracts files named lib*.so into it -
         * so the worker ships as `jniLibs/<abi>/libworker.so`, whatever it is
         * called in the Go module.
         */
        fun workerIn(nativeLibraryDir: String, name: String = "libworker.so"): String =
            File(nativeLibraryDir, name).path
    }
}
