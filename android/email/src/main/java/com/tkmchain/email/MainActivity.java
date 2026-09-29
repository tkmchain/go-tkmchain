package com.tkmchain.email;

import android.app.Activity;
import android.content.Intent;
import android.content.pm.PackageManager;
import android.graphics.Color;
import android.graphics.Typeface;
import android.graphics.drawable.GradientDrawable;
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
    private static final String DEFAULT_URL = "https://wallet.tkmchain.site/?app=email";
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
        root.setBackgroundColor(Color.rgb(9, 14, 20));
        root.setPadding(dp(16), dp(10), dp(16), 0);

        LinearLayout bar = new LinearLayout(this);
        bar.setGravity(Gravity.CENTER_VERTICAL);
        bar.setPadding(0, 0, 0, dp(10));
        TextView mark = new TextView(this);
        mark.setText("✉");
        mark.setGravity(Gravity.CENTER);
        mark.setTextSize(20);
        mark.setTextColor(Color.rgb(9, 20, 28));
        mark.setBackground(round(Color.rgb(88, 214, 193), Color.rgb(88, 214, 193), 12));
        bar.addView(mark, new LinearLayout.LayoutParams(dp(42), dp(42)));

        LinearLayout brand = new LinearLayout(this);
        brand.setOrientation(LinearLayout.VERTICAL);
        brand.setPadding(dp(10), 0, 0, 0);
        TextView title = new TextView(this);
        title.setText("TKM Mail");
        title.setTextColor(Color.WHITE);
        title.setTextSize(18);
        title.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        TextView subtitle = new TextView(this);
        subtitle.setText("Private EmailVM client");
        subtitle.setTextColor(Color.rgb(148, 164, 183));
        subtitle.setTextSize(11);
        brand.addView(title);
        brand.addView(subtitle);
        bar.addView(brand, new LinearLayout.LayoutParams(0, -2, 1));

        TextView torBadge = new TextView(this);
        torBadge.setText("TOR ONLY");
        torBadge.setGravity(Gravity.CENTER);
        torBadge.setTextColor(Color.rgb(88, 214, 193));
        torBadge.setTextSize(10);
        torBadge.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        torBadge.setPadding(dp(10), 0, dp(10), 0);
        torBadge.setBackground(round(Color.rgb(18, 44, 52), Color.rgb(46, 100, 111), 20));
        bar.addView(torBadge, new LinearLayout.LayoutParams(-2, dp(34)));

        Button reload = new Button(this);
        reload.setText("Reconnect");
        reload.setTextColor(Color.WHITE);
        reload.setTextSize(12);
        reload.setAllCaps(false);
        reload.setPadding(dp(12), 0, dp(12), 0);
        reload.setBackground(round(Color.rgb(37, 71, 103), Color.rgb(63, 112, 159), 10));
        reload.setOnClickListener(v -> startTorClient());
        LinearLayout.LayoutParams reloadParams = new LinearLayout.LayoutParams(-2, dp(40));
        reloadParams.setMargins(dp(8), 0, 0, 0);
        bar.addView(reload, reloadParams);
        root.addView(bar);

        LinearLayout hero = new LinearLayout(this);
        hero.setOrientation(LinearLayout.VERTICAL);
        hero.setPadding(dp(18), dp(18), dp(18), dp(18));
        hero.setBackground(round(Color.rgb(20, 37, 55), Color.rgb(44, 78, 108), 16));
        TextView eyebrow = new TextView(this);
        eyebrow.setText("EMAILVM  /  PRIVATE BY DEFAULT");
        eyebrow.setTextColor(Color.rgb(88, 214, 193));
        eyebrow.setTextSize(10);
        eyebrow.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        TextView headline = new TextView(this);
        headline.setText("Your address. Your keys. Your mail.");
        headline.setTextColor(Color.WHITE);
        headline.setTextSize(24);
        headline.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        headline.setPadding(0, dp(7), 0, dp(5));
        TextView copy = new TextView(this);
        copy.setText("Encrypted locally, registered on TKMChain, and transported through Tor. Signing stays in TKM Wallet.");
        copy.setTextColor(Color.rgb(182, 194, 207));
        copy.setTextSize(12);
        copy.setLineSpacing(2, 1.1f);
        hero.addView(eyebrow);
        hero.addView(headline);
        hero.addView(copy);
        LinearLayout.LayoutParams heroParams = new LinearLayout.LayoutParams(-1, -2);
        heroParams.setMargins(0, 0, 0, dp(10));
        root.addView(hero, heroParams);
        // The hosted wallet owns the email interface. Keep only the compact
        // Tor connection chrome above the WebView so Android and desktop show
        // the same wallet email screen.
        hero.setVisibility(android.view.View.GONE);

        LinearLayout statusPanel = new LinearLayout(this);
        statusPanel.setGravity(Gravity.CENTER_VERTICAL);
        statusPanel.setPadding(dp(13), dp(8), dp(13), dp(8));
        statusPanel.setBackground(round(Color.rgb(14, 23, 32), Color.rgb(42, 58, 73), 10));
        TextView statusLabel = new TextView(this);
        statusLabel.setText("CONNECTION");
        statusLabel.setTextColor(Color.rgb(148, 164, 183));
        statusLabel.setTextSize(10);
        statusLabel.setTypeface(Typeface.DEFAULT, Typeface.BOLD);
        statusPanel.addView(statusLabel, new LinearLayout.LayoutParams(-2, -2));
        status = new TextView(this);
        status.setTextColor(Color.rgb(230, 237, 243));
        status.setTextSize(12);
        status.setGravity(Gravity.END);
        status.setPadding(dp(8), 0, 0, 0);
        statusPanel.addView(status, new LinearLayout.LayoutParams(0, -2, 1));
        LinearLayout.LayoutParams statusParams = new LinearLayout.LayoutParams(-1, -2);
        statusParams.setMargins(0, 0, 0, dp(10));
        root.addView(statusPanel, statusParams);

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

    private int dp(int value) {
        return Math.round(value * getResources().getDisplayMetrics().density);
    }

    private GradientDrawable round(int fill, int stroke, int radius) {
        GradientDrawable drawable = new GradientDrawable();
        drawable.setColor(fill);
        drawable.setCornerRadius(dp(radius));
        drawable.setStroke(Math.max(1, dp(1)), stroke);
        return drawable;
    }

    private void startTorClient() {
        stopTorClient();
        web.stopLoading();
        web.loadData("<html><meta name=viewport content=\"width=device-width,initial-scale=1\"><body style=\"margin:0;background:#0a0e14;color:#f3f7fb;font:16px system-ui,sans-serif;padding:48px 24px\"><div style=\"color:#58d6c1;font-size:11px;font-weight:800;letter-spacing:1px\">EMAILVM / TOR GATE</div><h1 style=\"font-size:32px;line-height:1.05;margin:12px 0\">Preparing your private mailbox…</h1><p style=\"color:#94a4b7;line-height:1.6\">Waiting for Orbot to establish the encrypted route. No direct network fallback is used.</p></body></html>", "text/html", "UTF-8");
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
