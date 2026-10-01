// An Android frontend for the worker in ../../worker.
//
// The same rows as the macOS, WPF, GTK, iOS and browser examples: a Restart
// button, the job, the phase it is in, and how far through it is.
//
// The worker is an ordinary Go executable, shipped in the APK as
// jniLibs/arm64-v8a/libworker.so, because that is the only place Android will
// run one from.  There is no JNI and no C: Kotlin writes and reads JSON over a
// pipe, exactly as the Python and C# bindings do.
package com.imclaren.vero.example

import android.app.Activity
import android.graphics.Color
import android.os.Bundle
import android.view.Gravity
import android.view.ViewGroup.LayoutParams.MATCH_PARENT
import android.view.WindowInsets
import android.view.ViewGroup.LayoutParams.WRAP_CONTENT
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView
import com.imclaren.vero.NotRunning
import com.imclaren.vero.Vero
import com.imclaren.vero.VeroException
import org.json.JSONObject
import kotlin.concurrent.thread

class MainActivity : Activity() {

    private lateinit var vero: Vero
    private lateinit var jobs: LinearLayout
    private val rows = mutableMapOf<Int, Row>()

    private class Row(val name: TextView, val phase: TextView, val bar: ProgressBar)

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        jobs = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(32, 48, 32, 32)

            // An application targeting Android 15 is laid out edge to edge,
            // so its content starts behind the status bar rather than below
            // it.  Without this the first row is under the clock.
            setOnApplyWindowInsetsListener { view, insets ->
                val bars = insets.getInsets(WindowInsets.Type.systemBars())
                view.setPadding(32, bars.top + 48, 32, bars.bottom + 32)
                insets
            }
        }
        setContentView(jobs)

        vero = Vero(Vero.workerIn(applicationInfo.nativeLibraryDir))

        // latest draws a window that has just opened; the events after it keep
        // it current, and nothing polls.
        thread {
            val status = try {
                vero.latest()
            } catch (notYet: NotRunning) {
                null
            }
            status?.let { runOnUiThread { apply(it) } }
            vero.events { event -> event?.let { runOnUiThread { apply(it) } } }
        }
    }

    override fun onDestroy() {
        // Not required - the worker's standard input closes when this process
        // exits, and it goes with it - but it stops the work a moment sooner.
        thread { vero.stop() }
        super.onDestroy()
    }

    private fun apply(status: JSONObject) {
        val list = status.optJSONArray("jobs") ?: return
        for (i in 0 until list.length()) {
            val job = list.getJSONObject(i)
            val id = job.getInt("id")
            val row = rows.getOrPut(id) { addRow(id, job.getString("name")) }
            row.name.text = job.getString("name")
            row.phase.text = job.getString("phase")
            row.bar.progress = job.getInt("progress")
        }
    }

    /**
     * Asks the worker to run this job again.
     *
     * call() names the handler on the worker - "restartJob" is the
     * vero.UpdateWith in main.go - and the reply is the new status, so the
     * window redraws without waiting for the next event.
     */
    private fun restart(id: Int) {
        thread {
            try {
                vero.call("restartJob", JSONObject().put("id", id))
                    ?.let { status -> runOnUiThread { apply(status) } }
            } catch (notRunning: VeroException) {
                // Starting, restarting, or stopped: the next event redraws.
            }
        }
    }

    private fun addRow(id: Int, label: String): Row {
        val row = LinearLayout(this).apply {
            orientation = LinearLayout.HORIZONTAL
            gravity = Gravity.CENTER_VERTICAL
            layoutParams = LinearLayout.LayoutParams(MATCH_PARENT, WRAP_CONTENT)
        }

        val button = Button(this).apply {
            text = "Restart"
            setOnClickListener { restart(id) }
        }
        val name = TextView(this).apply {
            text = label
            setPadding(16, 0, 16, 0)
        }
        val phase = TextView(this).apply {
            setTextColor(Color.GRAY)
            setPadding(0, 0, 16, 0)
        }
        val bar = ProgressBar(this, null, android.R.attr.progressBarStyleHorizontal).apply {
            max = 100
            layoutParams = LinearLayout.LayoutParams(0, WRAP_CONTENT, 1f)
        }

        row.addView(button)
        row.addView(name)
        row.addView(phase)
        row.addView(bar)
        jobs.addView(row)
        return Row(name, phase, bar)
    }
}
