package com.tkmchain.node;

import android.app.Activity;
import android.content.Intent;
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

import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.InetSocketAddress;
import java.net.Socket;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.util.ArrayList;
import java.util.List;

public class MainActivity extends Activity {

    private static final String NODE_ASSET = "gtkm";
    private static final String PROVER_ASSET = "shielded-payout-prover";
    private static final String CXX_ASSET = "libc++_shared.so";
    private static final int GUI_PORT = 8081;
    private static final String GUI_URL = "http://127.0.0.1:" + GUI_PORT + "/";
    private static final String HEALTH_URL = "http://127.0.0.1:" + GUI_PORT + "/healthz";

    private final Handler main = new Handler(Looper.getMainLooper());
    private File binDir;
    private File dataDir;
    private File logFile;
    private boolean shuttingDown = false;

    private TextView status;
    private TextView statusDot;
    private WebView web;
    private ScrollView logScroll;
    private TextView logView;
    private Button restartBtn;
    private Thread probeThread;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // Executables are extracted from APK assets into an app-private directory.
        // Running directly from nativeLibraryDir is not reliable across Android
        // versions: the package linker may strip execute permissions from files
        // whose names end in .so, and ProcessBuilder then reports only EACCES.
        binDir = new File(getFilesDir(), "bin");
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
        try {
            if (!binDir.exists() && !binDir.mkdirs()) {
                throw new IOException("cannot create " + binDir);
            }
            installAsset(NODE_ASSET, true);
            installAsset(PROVER_ASSET, true);
            installAsset(CXX_ASSET, false);
            for (String name : new String[]{NODE_ASSET, PROVER_ASSET}) {
                File binary = new File(binDir, name);
                if (!binary.isFile() || !binary.canExecute() || binary.length() == 0) {
                    throw new IOException(name + " is not executable");
                }
            }
            return true;
        } catch (IOException | NoSuchAlgorithmException e) {
            appendLog("[wallet] executable setup failed: " + e.getMessage() + "\n");
            setStatus("Wallet installation is incomplete: " + e.getMessage());
            setStatusDot(Color.RED);
            return false;
        }
    }

    private void installAsset(String assetName, boolean executable)
            throws IOException, NoSuchAlgorithmException {
        File target = new File(binDir, assetName);
        String sourceDigest = digestAsset(assetName);
        if (target.isFile() && target.length() > 0 && sourceDigest.equals(digestFile(target))) {
            setPermissions(target, executable);
            return;
        }

        File temporary = new File(binDir, "." + assetName + ".tmp");
        try (InputStream in = getAssets().open(assetName);
             OutputStream out = new FileOutputStream(temporary)) {
            byte[] buffer = new byte[1024 * 1024];
            int n;
            while ((n = in.read(buffer)) != -1) out.write(buffer, 0, n);
            out.flush();
        }
        if (!sourceDigest.equals(digestFile(temporary))) {
            temporary.delete();
            throw new IOException("asset verification failed for " + assetName);
        }
        setPermissions(temporary, executable);
        if (target.exists() && !target.delete()) {
            temporary.delete();
            throw new IOException("cannot replace " + target);
        }
        if (!temporary.renameTo(target)) {
            temporary.delete();
            throw new IOException("cannot install " + target);
        }
        setPermissions(target, executable);
    }

    private String digestAsset(String assetName) throws IOException, NoSuchAlgorithmException {
        try (InputStream in = getAssets().open(assetName)) {
            return digest(in);
        }
    }

    private String digestFile(File file) throws IOException, NoSuchAlgorithmException {
        try (InputStream in = new FileInputStream(file)) {
            return digest(in);
        }
    }

    private String digest(InputStream in) throws IOException, NoSuchAlgorithmException {
        MessageDigest digest = MessageDigest.getInstance("SHA-256");
        byte[] buffer = new byte[1024 * 1024];
        int n;
        while ((n = in.read(buffer)) != -1) digest.update(buffer, 0, n);
        StringBuilder out = new StringBuilder(64);
        for (byte b : digest.digest()) out.append(String.format("%02x", b & 0xff));
        return out.toString();
    }

    private void setPermissions(File file, boolean executable) throws IOException {
        if (!file.setReadable(true, true) || !file.setWritable(true, true) ||
                (executable && !file.setExecutable(true, true))) {
            throw new IOException("cannot set permissions on " + file);
        }
    }

    private void startNode() {
        if (probeThread != null) probeThread.interrupt();

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
            Intent start = new Intent(this, NodeKeepAliveService.class)
                    .setAction(NodeKeepAliveService.ACTION_START)
                    .putStringArrayListExtra(NodeKeepAliveService.EXTRA_COMMAND, new ArrayList<>(cmd))
                    .putExtra(NodeKeepAliveService.EXTRA_WORK_DIR, dataDir.getAbsolutePath())
                    .putExtra(NodeKeepAliveService.EXTRA_LOG_FILE, logFile.getAbsolutePath())
                    .putExtra(NodeKeepAliveService.EXTRA_LIBRARY_DIR, binDir.getAbsolutePath());
            if (android.os.Build.VERSION.SDK_INT >= android.os.Build.VERSION_CODES.O) {
                startForegroundService(start);
            } else {
                startService(start);
            }
            probeThread = new Thread(this::probe, "node-probe");
            probeThread.setDaemon(true);
            probeThread.start();
        } catch (RuntimeException e) {
            setStatus("spawn failed: " + e.getMessage());
            setStatusDot(Color.RED);
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

    private void probe() {
        long started = System.currentTimeMillis();
        long deadline = System.currentTimeMillis() + 10 * 60 * 1000;
        while (!shuttingDown) {
            if (System.currentTimeMillis() > deadline) {
                main.post(() -> { if (!shuttingDown) setStatus("timeout waiting for dashboard"); setStatusDot(Color.RED); });
                return;
            }
            if (health()) {
                main.post(showWeb);
                return;
            }
            if (System.currentTimeMillis() - started > 3000) {
                String failure = serviceFailure();
                if (failure != null) {
                    main.post(() -> {
                        if (!shuttingDown) setStatus(failure + " — tap RESTART");
                        setStatusDot(Color.RED);
                    });
                    return;
                }
            }
            try { Thread.sleep(700); } catch (InterruptedException e) { return; }
        }
    }

    private String serviceFailure() {
        String text = readLogTail();
        int request = text.lastIndexOf("[tkm-service] node launch requested");
        if (request < 0) return null;
        String[] markers = {"[tkm-service] node launch failed:", "[tkm-service] node exited (code "};
        int started = text.lastIndexOf("[tkm-service] node started");
        for (String marker : markers) {
            int start = text.lastIndexOf(marker);
            if (start < request) continue;
            // An old process can finish just after a restart was requested.
            // Ignore that exit if the replacement process started afterward.
            if (marker.contains("node exited") && started >= request && start < started) continue;
            int end = text.indexOf('\n', start);
            if (end < 0) end = text.length();
            return text.substring(start, end).trim();
        }
        return null;
    }

    private String readLogTail() {
        try (FileInputStream in = new FileInputStream(logFile)) {
            byte[] all = new byte[16 * 1024];
            long skip = Math.max(0, logFile.length() - all.length);
            while (skip > 0) skip -= in.skip(skip);
            int n = in.read(all);
            return n > 0 ? new String(all, 0, n) : "";
        } catch (IOException ignored) {
            return "";
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
        if (probeThread != null) probeThread.interrupt();
        // NodeKeepAliveService owns the process so minimizing or rotating the
        // Activity cannot stop synchronization.
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

}
