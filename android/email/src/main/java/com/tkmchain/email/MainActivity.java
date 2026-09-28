package com.tkmchain.email;

import android.app.Activity;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.net.Uri;
import android.os.Bundle;
import android.os.Handler;
import android.os.Looper;
import android.view.Gravity;
import android.webkit.WebResourceError;
import android.webkit.WebResourceRequest;
import android.webkit.WebSettings;
import android.webkit.WebView;
import android.webkit.WebViewClient;
import android.widget.Button;
import android.widget.LinearLayout;
import android.widget.TextView;

import java.net.HttpURLConnection;
import java.net.InetSocketAddress;
import java.net.Proxy;
import java.net.URL;

/** Standalone EmailVM client. It has no wallet, node, or keystore access. */
public final class MainActivity extends Activity {
    private static final String DEFAULT_URL = "https://mail.tkmchain.site/";
    private static final String ORBOT_PACKAGE = "org.torproject.android";
    private static final String ORBOT_START_ACTION = "org.torproject.android.intent.action.START";
    private static final int[] TOR_SOCKS_PORTS = {9050, 9150};
    private static final long TOR_TIMEOUT_MS = 3 * 60 * 1000L;
    private static final String WALLET_PACKAGE = "com.tkmchain.node";

    private final Handler main = new Handler(Looper.getMainLooper());
    private TextView status;
    private WebView web;
    private TorWebViewProxy torProxy;
    private Thread torThread;
    private volatile boolean shuttingDown;

    @Override
    protected void onCreate(Bundle state) {
        super.onCreate(state);
        getWindow().setStatusBarColor(Color.rgb(11, 15, 23));
        getWindow().setNavigationBarColor(Color.rgb(11, 15, 23));
        buildView();
        startTorClient();
    }

    private void buildView() {
        LinearLayout root = new LinearLayout(this);
        root.setOrientation(LinearLayout.VERTICAL);
        root.setBackgroundColor(Color.rgb(11, 15, 23));

        LinearLayout bar = new LinearLayout(this);
        bar.setGravity(Gravity.CENTER_VERTICAL);
        bar.setPadding(20, 10, 12, 10);
        TextView title = new TextView(this);
        title.setText("TKM Email · Tor");
        title.setTextColor(Color.WHITE);
        title.setTextSize(18);
        title.setTypeface(null, android.graphics.Typeface.BOLD);
        bar.addView(title, new LinearLayout.LayoutParams(0, -2, 1));
        Button reload = new Button(this);
        reload.setText("Reconnect");
        reload.setOnClickListener(v -> startTorClient());
        bar.addView(reload, new LinearLayout.LayoutParams(-2, -2));
        root.addView(bar);

        status = new TextView(this);
        status.setTextColor(Color.LTGRAY);
        status.setTextSize(12);
        status.setPadding(20, 4, 20, 8);
        root.addView(status);

        web = new WebView(this);
        WebSettings settings = web.getSettings();
        settings.setJavaScriptEnabled(true);
        settings.setDomStorageEnabled(true);
        settings.setBuiltInZoomControls(false);
        settings.setAllowFileAccess(false);
        settings.setAllowContentAccess(false);
        web.setBackgroundColor(Color.rgb(11, 15, 23));
        web.setWebViewClient(new WebViewClient() {
            @Override
            public boolean shouldOverrideUrlLoading(WebView view, WebResourceRequest request) {
                return routeUrl(request.getUrl());
            }

            @Override
            public boolean shouldOverrideUrlLoading(WebView view, String url) {
                return routeUrl(Uri.parse(url));
            }

            @Override
            public void onPageStarted(WebView view, String url, android.graphics.Bitmap favicon) {
                status.setText("Connecting through Tor…");
            }

            @Override
            public void onPageFinished(WebView view, String url) {
                status.setText("TKM Email · Tor-only transport · review wallet actions before signing");
            }

            @Override
            public void onReceivedError(WebView view, WebResourceRequest request, WebResourceError error) {
                if (request.isForMainFrame()) {
                    status.setText("Unable to connect through Tor. Check Orbot, then tap Reconnect.");
                }
            }
        });
        root.addView(web, new LinearLayout.LayoutParams(-1, 0, 1));
        setContentView(root);
    }

    private void startTorClient() {
        stopTorClient();
        web.stopLoading();
        web.loadData("<html><meta name=viewport content=\"width=device-width,initial-scale=1\"><body style=\"margin:0;background:#111316;color:#eee;font:16px sans-serif;padding:48px 24px\"><h1 style=\"color:#f0b90b\">TKM Email</h1><p>Starting Tor…</p></body></html>", "text/html", "UTF-8");
        if (!requestOrbot()) {
            setStatus("Tor is unavailable. Install Orbot, then tap Reconnect.");
            return;
        }
        setStatus("Waiting for Orbot to connect to TKM mail…");
        torThread = new Thread(this::waitForTor, "email-tor-preflight");
        torThread.setDaemon(true);
        torThread.start();
    }

    private boolean requestOrbot() {
        try {
            getPackageManager().getApplicationInfo(ORBOT_PACKAGE, 0);
            sendBroadcast(new Intent(ORBOT_START_ACTION).setPackage(ORBOT_PACKAGE));
            Intent launch = getPackageManager().getLaunchIntentForPackage(ORBOT_PACKAGE);
            if (launch != null) startActivity(launch);
            return true;
        } catch (PackageManager.NameNotFoundException | RuntimeException e) {
            return false;
        }
    }

    private void waitForTor() {
        long deadline = System.currentTimeMillis() + TOR_TIMEOUT_MS;
        while (!shuttingDown && System.currentTimeMillis() < deadline) {
            for (int port : TOR_SOCKS_PORTS) {
                if (canReachRemote(port)) {
                    try {
                        TorWebViewProxy proxy = new TorWebViewProxy(DEFAULT_URL, port);
                        proxy.start();
                        main.post(() -> {
                            if (shuttingDown) {
                                proxy.close();
                                return;
                            }
                            torProxy = proxy;
                            status.setText("Connected through Tor");
                            web.loadUrl(proxy.url());
                        });
                    } catch (Exception e) {
                        main.post(() -> setStatus("Tor bridge failed. Tap Reconnect to try again."));
                    }
                    return;
                }
            }
            try {
                Thread.sleep(2000);
            } catch (InterruptedException e) {
                return;
            }
        }
        if (!shuttingDown) main.post(() -> setStatus("Tor did not reach TKM mail. Open Orbot and tap Reconnect."));
    }

    private boolean canReachRemote(int socksPort) {
        HttpURLConnection connection = null;
        try {
            Proxy proxy = new Proxy(Proxy.Type.SOCKS,
                    InetSocketAddress.createUnresolved("127.0.0.1", socksPort));
            connection = (HttpURLConnection) new URL(DEFAULT_URL).openConnection(proxy);
            connection.setConnectTimeout(8000);
            connection.setReadTimeout(8000);
            connection.setInstanceFollowRedirects(false);
            connection.setRequestMethod("HEAD");
            connection.getResponseCode();
            return true;
        } catch (Exception ignored) {
            return false;
        } finally {
            if (connection != null) connection.disconnect();
        }
    }

    private boolean routeUrl(Uri uri) {
        if (uri == null) return true;
        if ("tkmwallet".equalsIgnoreCase(uri.getScheme())) {
            handoffToWallet(uri);
            return true;
        }
        if (torProxy == null) return true;
        if ("http".equalsIgnoreCase(uri.getScheme()) && "localhost".equalsIgnoreCase(uri.getHost())) return false;
        if (!"http".equalsIgnoreCase(uri.getScheme()) && !"https".equalsIgnoreCase(uri.getScheme())) return true;
        String path = uri.getPath();
        if (path == null || path.isEmpty()) path = "/";
        if (uri.getQuery() != null) path += "?" + uri.getQuery();
        web.loadUrl(torProxy.localUrl(path));
        return true;
    }

    private void handoffToWallet(Uri uri) {
        try {
            String payload = uri.getQueryParameter("payload");
            if (payload == null || payload.length() > 64 * 1024) {
                setStatus("Wallet handoff rejected: invalid EmailVM request");
                return;
            }
            Intent intent = new Intent(Intent.ACTION_VIEW, uri);
            intent.setPackage(WALLET_PACKAGE);
            startActivity(intent);
            setStatus("EmailVM action sent to TKM Wallet for review");
        } catch (RuntimeException e) {
            setStatus("Install TKM Wallet to review this EmailVM action");
        }
    }

    private void stopTorClient() {
        if (torThread != null) torThread.interrupt();
        torThread = null;
        if (torProxy != null) torProxy.close();
        torProxy = null;
    }

    private void setStatus(String message) {
        if (status != null) status.setText(message);
    }

    @Override
    protected void onDestroy() {
        shuttingDown = true;
        stopTorClient();
        super.onDestroy();
    }

    @Override
    public void onBackPressed() {
        if (web != null && web.canGoBack()) web.goBack(); else super.onBackPressed();
    }
}
