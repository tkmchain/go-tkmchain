package com.tkmchain.node;

import android.app.Activity;
import android.content.Intent;
import android.content.pm.PackageManager;
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
import java.net.HttpURLConnection;
import java.net.InetSocketAddress;
import java.net.Proxy;
import java.net.Socket;
import java.net.URL;
import java.net.URLConnection;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.text.SimpleDateFormat;
import java.util.Date;
import java.util.ArrayList;
import java.util.List;
import java.util.Locale;
import java.util.TimeZone;

public class MainActivity extends Activity {

    private static final String NODE_BINARY = "libgtkm.so";
    private static final String PROVER_BINARY = "libshielded-payout-prover.so";
    private static final String BOOTSTRAP_URL = "https://tkmchain.site/tkm-mainnet.rlp.gz";
    private static final String BOOTSTRAP_SHA256 = "1a0137cdbef67217edbe6d4fb7001666897f477335461b07d2f03dcab2b2f779";
    private static final long MAX_BOOTSTRAP_BYTES = 512L << 30;
    private static final String ORBOT_PACKAGE = "org.torproject.android";
    private static final String ORBOT_START_ACTION = "org.torproject.android.intent.action.START";
    private static final String[] TOR_BOOTNODES = {
            "4aof7abdduh4vftejgdpdfqeosvxxco3xmpu4uqypnpdbi7wjuzfqhqd.onion",
            "eaoerarabizbzwbbawjrlcyawnrnoobj3ndy3oh627hwl5rbmedukoqd.onion"
    };
    private static final String TOR_BOOTNODE_ENODES =
            "enode://9f8ff5bda3629e9da4b2f1f4d4bd2385f38a382fa7f063b7f21c913411ef852dd224c7ee292450d587c3cbee5bd2d4a79999f0b9002070d7a559f8fe9a04baa2@4aof7abdduh4vftejgdpdfqeosvxxco3xmpu4uqypnpdbi7wjuzfqhqd.onion:3000?discport=0," +
            "enode://2c36e766ab52f04abfc129891b0d92d4d61dff6b8cf496910fd7046be7ca66afddc0086d527d9540003e766716a5337a2b866f8519708996fb8ff645e0b6b52e@eaoerarabizbzwbbawjrlcyawnrnoobj3ndy3oh627hwl5rbmedukoqd.onion:3000?discport=0";
    private static final int[] TOR_SOCKS_PORTS = {9050, 9150};
    private static final int TOR_P2P_PORT = 3000;
    private static final int GUI_PORT = 8081;
    private static final String GUI_URL = "http://127.0.0.1:" + GUI_PORT + "/";
    private static final String HEALTH_URL = "http://127.0.0.1:" + GUI_PORT + "/healthz";

    private final Handler main = new Handler(Looper.getMainLooper());
    private File nativeBinDir;
    private File dataDir;
    private File logFile;
    private boolean shuttingDown = false;

    private TextView status;
    private TextView statusDot;
    private WebView web;
    private ScrollView logScroll;
    private TextView logView;
    private Button restartBtn;
    private Button bootstrapBtn;
    private Thread probeThread;
    private Thread torThread;
    private Thread bootstrapThread;

    @Override
    protected void onCreate(Bundle savedInstanceState) {
        super.onCreate(savedInstanceState);

        // Executables are extracted by Android into nativeLibraryDir. On devices
        // with a noexec app-data mount, running a copy from filesDir fails with
        // EACCES even when chmod reports success.
        nativeBinDir = new File(getApplicationInfo().nativeLibraryDir);
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

        bootstrapBtn = new Button(this);
        bootstrapBtn.setText("Download bootstrap");
        bootstrapBtn.setTextColor(0xFFdceaff);
        bootstrapBtn.setMinHeight(dp(44));
        bootstrapBtn.setTextSize(11);
        bootstrapBtn.setAllCaps(false);
        bootstrapBtn.setContentDescription("Download the verified TKM chain bootstrap archive");
        android.graphics.drawable.GradientDrawable bootstrapBackground = new android.graphics.drawable.GradientDrawable();
        bootstrapBackground.setColor(0xFF243a57);
        bootstrapBackground.setCornerRadius(dp(10));
        bootstrapBtn.setBackground(bootstrapBackground);
        bootstrapBtn.setOnClickListener(v -> downloadBootstrap());
        bar.addView(bootstrapBtn);
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
            for (String name : new String[]{NODE_BINARY, PROVER_BINARY}) {
                File binary = new File(nativeBinDir, name);
                if (!binary.isFile() || !binary.canExecute() || binary.length() == 0) {
                    throw new IOException(name + " is not executable in " + nativeBinDir);
                }
            }
            return true;
        } catch (IOException e) {
            appendLog("[wallet] executable setup failed: " + e.getMessage() + "\n");
            setStatus("Wallet installation is incomplete: " + e.getMessage());
            setStatusDot(Color.RED);
            return false;
        }
    }

    private void startNode() {
        if (probeThread != null) probeThread.interrupt();
        if (torThread != null) torThread.interrupt();

        setStatusDot(Color.YELLOW);
        setStatus("connecting to the TKM Tor network\u2026");
        main.removeCallbacks(showWeb);
        web.stopLoading();
        web.loadData("<html><meta name=viewport content=\"width=device-width,initial-scale=1\"><body style=\"margin:0;background:#111316;color:#eee;font:16px sans-serif;padding:48px 24px\"><h1 style=\"color:#f0b90b\">TKM Wallet</h1><h2>Starting your wallet</h2><p style=\"color:#a9afb9;line-height:1.7\">Connecting to your local node. The first setup downloads verified proof files and can take a few minutes.</p></body></html>", "text/html", "UTF-8");

        if (!requestOrbot()) {
            setStatus("Tor is unavailable. Install Orbot, then tap RESTART.");
            setStatusDot(Color.RED);
            return;
        }
        torThread = new Thread(this::waitForTorThenStart, "tor-preflight");
        torThread.setDaemon(true);
        torThread.start();
    }

    private boolean requestOrbot() {
        try {
            getPackageManager().getApplicationInfo(ORBOT_PACKAGE, 0);
        } catch (PackageManager.NameNotFoundException e) {
            return false;
        }
        try {
            sendBroadcast(new Intent(ORBOT_START_ACTION).setPackage(ORBOT_PACKAGE));
            Intent launch = getPackageManager().getLaunchIntentForPackage(ORBOT_PACKAGE);
            if (launch != null) startActivity(launch);
            return true;
        } catch (RuntimeException e) {
            appendLog("[wallet] could not start Orbot: " + e.getMessage() + "\n");
            return false;
        }
    }

    private void waitForTorThenStart() {
        long deadline = System.currentTimeMillis() + 3 * 60 * 1000;
        while (!shuttingDown && System.currentTimeMillis() < deadline) {
            for (int port : TOR_SOCKS_PORTS) {
                if (canReachOnion(port)) {
                    String proxy = "socks5://127.0.0.1:" + port;
                    main.post(() -> launchNode(proxy));
                    return;
                }
            }
            main.post(() -> setStatus("waiting for Tor to reach a TKM onion peer\u2026"));
            try {
                Thread.sleep(2000);
            } catch (InterruptedException e) {
                return;
            }
        }
        main.post(() -> {
            if (!shuttingDown) setStatus("Tor did not reach a TKM onion peer. Open Orbot and tap RESTART.");
            setStatusDot(Color.RED);
        });
    }

    private boolean canReachOnion(int socksPort) {
        Proxy proxy = new Proxy(Proxy.Type.SOCKS,
                InetSocketAddress.createUnresolved("127.0.0.1", socksPort));
        for (String onion : TOR_BOOTNODES) {
            try (Socket socket = new Socket(proxy)) {
                socket.connect(InetSocketAddress.createUnresolved(onion, TOR_P2P_PORT), 8000);
                return true;
            } catch (IOException ignored) {
                // Try the second bootstrap onion and then the alternate SOCKS port.
            }
        }
        return false;
    }

    /** Downloads the official, hash-checked archive into the node instance directory. */
    private void downloadBootstrap() {
        if (bootstrapThread != null && bootstrapThread.isAlive()) return;
        bootstrapBtn.setEnabled(false);
        setStatusDot(Color.YELLOW);
        setStatus("connecting to Tor before downloading bootstrap\u2026");
        appendLog("[wallet] bootstrap download requested\n");
        if (!requestOrbot()) {
            bootstrapBtn.setEnabled(true);
            setStatus("Tor is unavailable. Install Orbot, then try again.");
            setStatusDot(Color.RED);
            return;
        }
        bootstrapThread = new Thread(() -> {
            try {
                int socksPort = waitForTorPort();
                if (socksPort == 0) {
                    throw new IOException("Tor could not reach a TKM onion peer");
                }
                File target = downloadBootstrapArchive(socksPort);
                appendLog("[wallet] bootstrap saved to " + target.getAbsolutePath() + "\n");
                main.post(() -> {
                    if (shuttingDown) return;
                    setStatus("bootstrap saved to " + target.getName());
                    setStatusDot(Color.GREEN);
                });
            } catch (InterruptedException e) {
                Thread.currentThread().interrupt();
            } catch (Exception e) {
                appendLog("[wallet] bootstrap download failed: " + e.getMessage() + "\n");
                main.post(() -> {
                    if (shuttingDown) return;
                    setStatus("bootstrap download failed: " + e.getMessage());
                    setStatusDot(Color.RED);
                });
            } finally {
                main.post(() -> {
                    if (!shuttingDown) bootstrapBtn.setEnabled(true);
                });
            }
        }, "bootstrap-download");
        bootstrapThread.setDaemon(true);
        bootstrapThread.start();
    }

    private int waitForTorPort() throws InterruptedException {
        long deadline = System.currentTimeMillis() + 3 * 60 * 1000;
        while (!shuttingDown && System.currentTimeMillis() < deadline) {
            for (int port : TOR_SOCKS_PORTS) {
                if (canReachOnion(port)) return port;
            }
            main.post(() -> {
                if (!shuttingDown) setStatus("waiting for Tor to reach a TKM onion peer\u2026");
            });
            Thread.sleep(2000);
        }
        return 0;
    }

    private File downloadBootstrapArchive(int socksPort) throws IOException, NoSuchAlgorithmException {
        File bootstrapDir = new File(dataDir, "gtkm/bootstrap");
        if (!bootstrapDir.isDirectory() && !bootstrapDir.mkdirs()) {
            throw new IOException("cannot create " + bootstrapDir);
        }
        File temporary = File.createTempFile(".chain-", ".part", bootstrapDir);
        boolean installed = false;
        try {
            URLConnection connection = new URL(BOOTSTRAP_URL).openConnection(new Proxy(
                    Proxy.Type.SOCKS, InetSocketAddress.createUnresolved("127.0.0.1", socksPort)));
            connection.setConnectTimeout(15000);
            connection.setReadTimeout(60000);
            if (connection instanceof HttpURLConnection) {
                HttpURLConnection http = (HttpURLConnection) connection;
                http.setInstanceFollowRedirects(false);
                int code = http.getResponseCode();
                if (code < 200 || code >= 300) {
                    throw new IOException("bootstrap server returned HTTP " + code);
                }
            }
            long expectedSize = connection.getContentLengthLong();
            if (expectedSize > MAX_BOOTSTRAP_BYTES) {
                throw new IOException("bootstrap archive is too large");
            }

            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            long total = 0;
            long nextStatus = 0;
            byte[] buffer = new byte[1024 * 1024];
            try (InputStream in = connection.getInputStream(); FileOutputStream out = new FileOutputStream(temporary)) {
                int read;
                while ((read = in.read(buffer)) != -1) {
                    total += read;
                    if (total > MAX_BOOTSTRAP_BYTES) throw new IOException("bootstrap archive is too large");
                    digest.update(buffer, 0, read);
                    out.write(buffer, 0, read);
                    if (total >= nextStatus) {
                        nextStatus = total + (16L << 20);
                        long downloaded = total;
                        main.post(() -> {
                            if (!shuttingDown) setStatus("downloading bootstrap (" + formatBytes(downloaded) + ")\u2026");
                        });
                    }
                }
                out.getFD().sync();
            }
            if (total == 0) throw new IOException("bootstrap archive is empty");
            String actual = hex(digest.digest());
            if (!BOOTSTRAP_SHA256.equalsIgnoreCase(actual)) {
                throw new IOException("bootstrap SHA-256 mismatch");
            }
            SimpleDateFormat stamp = new SimpleDateFormat("yyyyMMdd'T'HHmmss'Z'", Locale.US);
            stamp.setTimeZone(TimeZone.getTimeZone("UTC"));
            File target = new File(bootstrapDir, "chain-" + stamp.format(new Date()) + "-" + System.currentTimeMillis() + ".rlp.gz");
            if (!temporary.renameTo(target)) throw new IOException("cannot install " + target);
            installed = true;
            return target;
        } finally {
            if (!installed) temporary.delete();
        }
    }

    private String formatBytes(long bytes) {
        if (bytes < (1L << 20)) return (bytes / 1024) + " KiB";
        if (bytes < (1L << 30)) return String.format(Locale.US, "%.1f MiB", bytes / (double) (1L << 20));
        return String.format(Locale.US, "%.2f GiB", bytes / (double) (1L << 30));
    }

    private String hex(byte[] bytes) {
        StringBuilder out = new StringBuilder(bytes.length * 2);
        for (byte b : bytes) out.append(String.format(Locale.US, "%02x", b & 0xff));
        return out.toString();
    }

    private List<String> nodeCommand(String proxy) {
        File bin = new File(nativeBinDir, NODE_BINARY);
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
        cmd.add("--p2p.tor-socks5");
        cmd.add(proxy);
        cmd.add("--privacy.onion-only=false");
        cmd.add("--bootnodes");
        cmd.add(TOR_BOOTNODE_ENODES);
        cmd.add("--nodiscover");
        cmd.add("--nat");
        cmd.add("none");
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
        cmd.add(new File(nativeBinDir, PROVER_BINARY).getAbsolutePath());
        cmd.add("--tkmprover.config");
        cmd.add(new File(dataDir, "tkmprover/config.json").getAbsolutePath());
        return cmd;
    }

    private void launchNode(String proxy) {
        List<String> cmd = nodeCommand(proxy);
        setStatus("Tor connected \u2014 starting node\u2026");
        try {
            Intent start = new Intent(this, NodeKeepAliveService.class)
                    .setAction(NodeKeepAliveService.ACTION_START)
                    .putStringArrayListExtra(NodeKeepAliveService.EXTRA_COMMAND, new ArrayList<>(cmd))
                    .putExtra(NodeKeepAliveService.EXTRA_WORK_DIR, dataDir.getAbsolutePath())
                    .putExtra(NodeKeepAliveService.EXTRA_LOG_FILE, logFile.getAbsolutePath())
                    .putExtra(NodeKeepAliveService.EXTRA_LIBRARY_DIR, nativeBinDir.getAbsolutePath());
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
        if (torThread != null) torThread.interrupt();
        if (bootstrapThread != null) bootstrapThread.interrupt();
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
