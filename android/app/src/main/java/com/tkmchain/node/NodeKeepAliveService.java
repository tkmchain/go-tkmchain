package com.tkmchain.node;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.Service;
import android.content.Intent;
import android.os.Build;
import android.os.IBinder;
import android.os.PowerManager;

import java.io.File;
import java.io.FileOutputStream;
import java.io.IOException;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.Arrays;
import java.util.List;

/** Keeps the local node process important while the wallet UI is minimized. */
public final class NodeKeepAliveService extends Service {
    private static final String CHANNEL_ID = "node_sync";
    public static final String ACTION_START = "com.tkmchain.node.START";
    public static final String ACTION_STOP = "com.tkmchain.node.STOP";
    public static final String EXTRA_COMMAND = "command";
    public static final String EXTRA_WORK_DIR = "workDir";
    public static final String EXTRA_LOG_FILE = "logFile";
    public static final String EXTRA_LIBRARY_DIR = "libraryDir";
    private static final String PREFS = "node_service";
    private static final String PREF_COMMAND = "command";
    private static final String PREF_WORK_DIR = "workDir";
    private static final String PREF_LOG_FILE = "logFile";
    private static final String PREF_LIBRARY_DIR = "libraryDir";

    private PowerManager.WakeLock wakeLock;
    private final Object processLock = new Object();
    private Process nodeProcess;
    private Thread waitThread;

    @Override
    public void onCreate() {
        super.onCreate();
        NotificationManager manager = getSystemService(NotificationManager.class);
        if (Build.VERSION.SDK_INT >= Build.VERSION_CODES.O) {
            manager.createNotificationChannel(new NotificationChannel(
                    CHANNEL_ID, "Wallet synchronization", NotificationManager.IMPORTANCE_LOW));
        }
        Notification notification = new Notification.Builder(this, CHANNEL_ID)
                .setSmallIcon(android.R.drawable.stat_sys_download)
                .setContentTitle("TKM wallet syncing")
                .setContentText("Node synchronization continues in the background")
                .setOngoing(true)
                .build();
        startForeground(1001, notification);

        PowerManager power = getSystemService(PowerManager.class);
        wakeLock = power.newWakeLock(PowerManager.PARTIAL_WAKE_LOCK, "tkm:node-sync");
        wakeLock.setReferenceCounted(false);
        wakeLock.acquire();
    }

    @Override
    public int onStartCommand(Intent intent, int flags, int startId) {
        if (intent != null && ACTION_STOP.equals(intent.getAction())) {
            clearSavedCommand();
            stopNode();
            stopSelfResult(startId);
            return START_NOT_STICKY;
        }

        ArrayList<String> command = intent == null ? null :
                intent.getStringArrayListExtra(EXTRA_COMMAND);
        String workDir = intent == null ? null : intent.getStringExtra(EXTRA_WORK_DIR);
        String logFile = intent == null ? null : intent.getStringExtra(EXTRA_LOG_FILE);
        String libraryDir = intent == null ? null : intent.getStringExtra(EXTRA_LIBRARY_DIR);
        if (command == null || command.isEmpty()) {
            command = readSavedCommand();
            workDir = prefs().getString(PREF_WORK_DIR, null);
            logFile = prefs().getString(PREF_LOG_FILE, null);
            libraryDir = prefs().getString(PREF_LIBRARY_DIR, null);
        }
        if (command != null && !command.isEmpty() && workDir != null && logFile != null && libraryDir != null) {
            saveCommand(command, workDir, logFile, libraryDir);
            startNode(command, workDir, logFile, libraryDir);
        } else {
            appendLog(null, "[tkm-service] no saved node command; waiting for wallet\n");
        }
        // A foreground service is restarted with a null intent after process
        // reclaim. The command is persisted so synchronization resumes.
        return START_STICKY;
    }

    @Override
    public void onDestroy() {
        stopNode();
        if (wakeLock != null && wakeLock.isHeld()) wakeLock.release();
        super.onDestroy();
    }

    @Override
    public IBinder onBind(Intent intent) {
        return null;
    }

    private void startNode(List<String> command, String workDir, String logFile, String libraryDir) {
        stopNode();
        File log = new File(logFile);
        File dir = new File(workDir);
        if (!dir.isDirectory() && !dir.mkdirs()) {
            appendLog(log, "[tkm-service] cannot create node directory " + dir + "\n");
            return;
        }
        appendLog(log, "[tkm-service] node launch requested\n");
        try {
            ProcessBuilder builder = new ProcessBuilder(command);
            builder.directory(dir);
            builder.environment().put("LD_LIBRARY_PATH", libraryDir);
            builder.redirectErrorStream(true);
            builder.redirectOutput(ProcessBuilder.Redirect.appendTo(log));
            Process process = builder.start();
            synchronized (processLock) {
                nodeProcess = process;
            }
            appendLog(log, "[tkm-service] node started\n");
            waitThread = new Thread(() -> waitForNode(process, log), "tkm-node-wait");
            waitThread.setDaemon(true);
            waitThread.start();
        } catch (IOException e) {
            appendLog(log, "[tkm-service] node launch failed: " + e.getMessage() + "\n");
        }
    }

    private void waitForNode(Process process, File log) {
        int exit;
        try {
            exit = process.waitFor();
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            return;
        }
        synchronized (processLock) {
            if (nodeProcess == process) nodeProcess = null;
        }
        appendLog(log, "[tkm-service] node exited (code " + exit + ")\n");
    }

    private void stopNode() {
        Process process;
        synchronized (processLock) {
            process = nodeProcess;
            nodeProcess = null;
        }
        if (process == null) return;
        process.destroy();
        try {
            if (!process.waitFor(2, java.util.concurrent.TimeUnit.SECONDS)) process.destroyForcibly();
        } catch (InterruptedException e) {
            Thread.currentThread().interrupt();
            process.destroyForcibly();
        }
    }

    private void saveCommand(List<String> command, String workDir, String logFile, String libraryDir) {
        prefs().edit()
                .putString(PREF_COMMAND, String.join("\n", command))
                .putString(PREF_WORK_DIR, workDir)
                .putString(PREF_LOG_FILE, logFile)
                .putString(PREF_LIBRARY_DIR, libraryDir)
                .apply();
    }

    private ArrayList<String> readSavedCommand() {
        String encoded = prefs().getString(PREF_COMMAND, "");
        if (encoded.isEmpty()) return null;
        return new ArrayList<>(Arrays.asList(encoded.split("\\n", -1)));
    }

    private void clearSavedCommand() {
        prefs().edit().clear().apply();
    }

    private android.content.SharedPreferences prefs() {
        return getSharedPreferences(PREFS, MODE_PRIVATE);
    }

    private void appendLog(File file, String message) {
        if (file == null) return;
        try {
            File parent = file.getParentFile();
            if (parent != null) parent.mkdirs();
            try (FileOutputStream out = new FileOutputStream(file, true)) {
                out.write(message.getBytes(StandardCharsets.UTF_8));
            }
        } catch (IOException ignored) {
        }
    }
}
