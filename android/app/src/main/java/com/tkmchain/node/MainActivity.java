package com.tkmchain.node;

import android.app.Activity;
import android.content.Intent;
import android.content.res.AssetManager;
import android.graphics.Color;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.util.TypedValue;
import android.view.Gravity;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.ScrollView;
import android.widget.TextView;

import java.io.BufferedReader;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.InputStreamReader;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.util.ArrayList;
import java.util.List;

public class MainActivity extends Activity {

    private static final String NODE_ASSET = "libgtkm.so";
    private static final String PROVER_ASSET = "libtkmprover.so";
    private static final String CXX_ASSET = "libc++_shared.so";
    private static final int GUI_PORT = 8081;
    private static final String GUI_URL = "http://127.0.0.1:" + GUI_PORT + "/";
    private static final String HEALTH_URL = "http://127.0.0.1:" + GUI_PORT + "/healthz";

    private final Handler main = new Handler(Looper.getMainLooper());
    private final Object procLock = new Object();

    private File binDir;
    private File dataDir;
    private File logFile;
    private Process nodeProc;
    private boolean shuttingDown = false;

    private TextView status;
    private TextView statusDot;
    private WebView web;
    private ScrollView logScroll;
    private TextView logView;
    private Button restartBtn;
    private Thread pumpThread;
    private Thread probeThread;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        Intent keepAlive = new Intent(this, NodeKeepAliveService.class);
        if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.O) {
            startForegroundService(keepAlive);
        } else {
            startService(keepAlive);
        }

        binDir = new File(getApplicationInfo().nativeLibraryDir);
        dataDir = new File(getFilesDir(), "node");
        logFile = new File(getFilesDir(), "node.log");

        dataDir.mkdirs();

        getWindow().setFlags(android.view.WindowManager.LayoutParams.FLAG_SECURE, android.view.WindowManager.LayoutParams.FLAG_SECURE);
        buildUi();
        if (prepareAssets()) startNode();
    }

    private void buildUi() {
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setBackgroundColor(0xFF0b1019);
        getWindow().setStatusBarColor(0xFF101925);
        getWindow().setNavigationBarColor(0xFF0b1019);
        getWindow().setSoftInputMode(android.view.WindowManager.LayoutParams.SOFT_INPUT_ADJUST_RESIZE);

        LinearLayout bar = new LinearLayout(this);
        bar.setOrientation(LinearLayout.HORIZONTAL);
        bar.setGravity(Gravity.CENTER_VERTICAL);
        bar.setPadding(dp(16), dp(8), dp(12), dp(8));
        bar.setBackgroundColor(0xFF101925);

        statusDot = new TextView(this);
        statusDot.setText("\u25CF");
        statusDot.setTextSize(14);
        statusDot.setTextColor(Color.GRAY);
        bar.addView(statusDot);

        status = new TextView(this);
        status.setTextSize(12);
        status.setTextColor(0xFFafbdd0);
        status.setEllipsize(null);
        status.setSingleLine(false);
        status.setMaxLines(2);
        status.setEllipsize(android.text.TextUtils.TruncateAt.END);
        status.setContentDescription("Node status. Tap to show or hide diagnostics.");
        status.setPadding(dp(8), 0, dp(8), 0);
        LinearLayout.LayoutParams sp = new LinearLayout.LayoutParams(0, LinearLayout.LayoutParams.WRAP_CONTENT, 1f);
        bar.addView(status, sp);

        restartBtn = new Button(this);
        restartBtn.setText("Restart");
        restartBtn.setTextColor(0xFFdceaff);
        restartBtn.setMinHeight(dp(44));
        android.graphics.drawable.GradientDrawable restartBackground = new android.graphics.drawable.GradientDrawable();
        restartBackground.setColor(0xFF243a57);
        restartBackground.setCornerRadius(dp(10));
        restartBtn.setBackground(restartBackground);
        restartBtn.setTextSize(11);
        restartBtn.setAllCaps(false);
        restartBtn.setOnClickListener(v -> new android.app.AlertDialog.Builder(this)
                .setTitle("Restart node?")
                .setMessage("Your wallet will reconnect when the node is ready.")
                .setNegativeButton("Cancel", null)
                .setPositiveButton("Restart", (dialog, which) -> startNode()).show());
        bar.addView(restartBtn);
        root.addView(bar);

        web = new WebView(this);
        WebSettings s = web.getSettings();
        s.setJavaScriptEnabled(true);
        s.setDomStorageEnabled(true);
        s.setAllowFileAccess(false);
        s.setAllowContentAccess(false);
        s.setCacheMode(WebSettings.LOAD_DEFAULT);
        s.setMediaPlaybackRequiresUserGesture(true);
        s.setBuiltInZoomControls(false);
        s.setDisplayZoomControls(false);
        web.setBackgroundColor(0xFF0b1019);
        web.setOverScrollMode(android.view.View.OVER_SCROLL_NEVER);
        web.setWebViewClient(new WebViewClient());
        root.addView(web, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f));

        logScroll = new ScrollView(this);
        logView = new TextView(this);
        logView.setTextColor(0xFFc9d1d9);
        logView.setTextSize(12);
        logView.setPadding(dp(16), dp(12), dp(16), dp(12));
        logView.setTextIsSelectable(true);
        logView.setTypeface(android.graphics.Typeface.MONOSPACE);
        logScroll.addView(logView);
        logScroll.setVisibility(ScrollView.GONE);
        root.addView(logScroll, new LinearLayout.LayoutParams(
                LinearLayout.LayoutParams.MATCH_PARENT, 0, 1f));

        status.setOnClickListener(v -> {
            boolean show = logScroll.getVisibility() != ScrollView.VISIBLE;
            logScroll.setVisibility(show ? ScrollView.VISIBLE : ScrollView.GONE);
            if (show) refreshLogView();
        });

        setContentView(root);
    }

    private boolean prepareAssets() {
        for (String name : new String[]{NODE_ASSET, PROVER_ASSET}) {
            File binary = new File(binDir, name);
            if (!binary.isFile() || !binary.canExecute()) {
                setStatus("Wallet installation is incomplete. Install the ARM64 APK again without clearing wallet data.");
                setStatusDot(Color.RED);
                return false;
            }
        }
        return true;
    }

    private void startNode() {
        if (nodeProc != null) {
            nodeProc.destroy();
            nodeProc = null;
        }
        if (probeThread != null) probeThread.interrupt();
        if (pumpThread != null) pumpThread.interrupt();

        File bin = new File(binDir, NODE_ASSET);
        List<String> cmd = new ArrayList<>();
        cmd.add(bin.getAbsolutePath());
        cmd.add("gui");
        cmd.add("--gui.host");
        cmd.add("127.0.0.1");
        cmd.add("--gui.port");
        cmd.add(String.valueOf(GUI_PORT));
        cmd.add("--datadir");
        cmd.add(dataDir.getAbsolutePath());
        cmd.add("--networkid");
        cmd.add("8979");
        cmd.add("--syncmode");
        cmd.add("snap");
        cmd.add("--cache");
        cmd.add("512");
        cmd.add("--http");
        cmd.add("--http.addr");
        cmd.add("127.0.0.1");
        cmd.add("--http.port");
        cmd.add("8545");
        cmd.add("--http.vhosts");
        cmd.add("localhost");
        cmd.add("--http.api");
        cmd.add("eth,net,web3,tkm,tkmprivacy");
        cmd.add("--tkmprover");
        cmd.add("--tkmprover.bin");
        cmd.add(new File(binDir, PROVER_ASSET).getAbsolutePath());
        cmd.add("--tkmprover.config");
        cmd.add(new File(dataDir, "tkmprover/config.json").getAbsolutePath());

        setStatusDot(Color.YELLOW);
        setStatus("starting node\u2026");
        main.removeCallbacks(showWeb);
        web.stopLoading();
        web.loadData("<html><meta name=viewport content=\"width=device-width,initial-scale=1\"><body style=\"margin:0;background:#111316;color:#eee;font:16px sans-serif;padding:48px 24px\"><h1 style=\"color:#f0b90b\">TKM Wallet</h1><h2>Starting your wallet</h2><p style=\"color:#a9afb9;line-height:1.7\">Connecting to your local node. The first setup downloads verified proof files and can take a few minutes.</p></body></html>", "text/html", "UTF-8");

        try {
            Process p = launch(cmd, bin);
            synchronized (procLock) {
                nodeProc = p;
            }
            pumpThread = new Thread(() -> pump(p), "node-log");
            pumpThread.setDaemon(true);
            pumpThread.start();
            probeThread = new Thread(() -> probe(p), "node-probe");
            probeThread.setDaemon(true);
            probeThread.start();
        } catch (IOException e) {
            setStatus("spawn failed: " + e.getMessage());
            setStatusDot(Color.RED);
        }
    }

    private Process launch(List<String> cmd, File bin) throws IOException {
        // APK native libraries are installed into Android's executable code directory.
        // Never copy executable code into writable app data or invoke linker64.
        ProcessBuilder pb = new ProcessBuilder(cmd);
        pb.environment().put("LD_LIBRARY_PATH", binDir.getAbsolutePath());
        pb.directory(dataDir);
        pb.redirectErrorStream(true);
        return pb.start();
    }

    private void pump(Process p) {
        try (BufferedReader r = new BufferedReader(new InputStreamReader(p.getInputStream()))) {
            StringBuilder tail = new StringBuilder();
            String line;
            while ((line = r.readLine()) != null) {
                String l = line + "\n";
                synchronized (tailBufferLock) {
                    tailBuffer.add(l);
                    while (tailBuffer.size() > 400) tailBuffer.remove(0);
                }
                appendLog(l);
                if (tail.length() > 600) tail.delete(0, tail.length() / 2);
                tail.append(line).append('\n');
                String shown = tail.toString();
                // Detailed output belongs in diagnostics, not the wallet header.
            }
        } catch (IOException ignored) {
        }
        if (!shuttingDown && nodeProc == p) {
            int exit;
            try { exit = p.waitFor(); } catch (InterruptedException e) { Thread.currentThread().interrupt(); return; }
            main.post(() -> {
                if (!shuttingDown) {
                    setStatus("node exited (code " + exit + ") \u2014 tap RESTART");
                    setStatusDot(Color.RED);
                }
                if (nodeProc == p) nodeProc = null;
            });
        }
    }

    private synchronized void appendLog(String s) {
        try (FileOutputStream fos = new FileOutputStream(logFile, true)) {
            fos.write(s.getBytes());
        } catch (IOException ignored) {
        }
        refreshLogView();
    }

    private void refreshLogView() {
        if (logScroll.getVisibility() != ScrollView.VISIBLE) return;
        StringBuilder sb = new StringBuilder();
        FileInputStream in = null;
        try {
            in = new FileInputStream(logFile);
            byte[] buf = new byte[8192];
            int n;
            while ((n = in.read(buf)) > 0) {
                String s = new String(buf, 0, n);
                sb.append(s);
                if (sb.length() > 600_000) break;
            }
        } catch (IOException ignored) {
        } finally {
            if (in != null) try { in.close(); } catch (IOException ignored) {}
        }
        final String txt = sb.toString();
        main.post(() -> {
            logView.setText(txt);
            logScroll.post(() -> logScroll.fullScroll(ScrollView.FOCUS_DOWN));
        });
    }

    private void probe(Process expected) {
        long deadline = System.currentTimeMillis() + 10 * 60 * 1000;
        while (!shuttingDown && nodeProc == expected && expected.isAlive()) {
            if (System.currentTimeMillis() > deadline) {
                main.post(() -> { if (!shuttingDown) setStatus("timeout waiting for dashboard"); setStatusDot(Color.RED); });
                return;
            }
            if (health()) {
                main.post(() -> { if (nodeProc == expected) showWeb.run(); });
                return;
            }
            try { Thread.sleep(700); } catch (InterruptedException e) { return; }
        }
    }

    private boolean health() {
        try {
            Socket sock = new Socket();
            sock.connect(new InetSocketAddress("127.0.0.1", GUI_PORT), 600);
            sock.close();
            HttpURLConnection c = (HttpURLConnection) new java.net.URL(HEALTH_URL).openConnection();
            c.setConnectTimeout(600);
            c.setReadTimeout(600);
            int code = c.getResponseCode();
            c.disconnect();
            return code == 200;
        } catch (IOException e) {
            return false;
        }
    }

    private final Runnable showWeb = () -> {
        if (shuttingDown) return;
        setStatus("node is running \u2014 dashboard loading\u2026");
        setStatusDot(Color.GREEN);
        web.loadUrl(GUI_URL);
    };

    @Override
    public void onBackPressed() {
        if (web.canGoBack()) {
            web.goBack();
        } else {
            super.onBackPressed();
        }
    }

    @Override
    protected void onDestroy() {
        shuttingDown = true;
        Process p;
        synchronized (procLock) {
            p = nodeProc;
            nodeProc = null;
        }
        if (p != null) p.destroy();
        if (probeThread != null) probeThread.interrupt();
        if (pumpThread != null) pumpThread.interrupt();
        super.onDestroy();
    }

    private void setStatus(final String s) {
        main.post(() -> status.setText(s));
    }

    private void setStatusDot(final int color) {
        main.post(() -> statusDot.setTextColor(color));
    }

    private int dp(int v) {
        return (int) TypedValue.applyDimension(TypedValue.COMPLEX_UNIT_DIP, v, getResources().getDisplayMetrics());
    }

    private final java.util.List<String> tailBuffer = new java.util.LinkedList<>();
    private final Object tailBufferLock = new Object();
}
