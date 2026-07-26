// rustbench encodes images with the webp-rust library (the Rust original this Go
// port is based on) across three modes, emitting CSV lines compatible with the
// Go webpbench tool:
//
//   engine,mode,file,width,height,bytes,psnr_db,iters,ms_per_op
//
// psnr_db scores the encoder's own output against the pixels it was handed, and
// is "-" for lossless.
//
// With --mem it runs the peak-RSS pass instead, emitting:
//
//   engine,mode,file,width,height,megapixels,peak_rss_mib,mib_per_mp
//
// Each measurement gets its own process (this binary, re-executed with
// --mem-one) that decodes the source image, encodes it once, and reports its own
// ru_maxrss, matching what benchmark/webpbench does for the Go engines.
//
// Usage: rustbench <dir> [budget_ms] [min_iters] [max_iters]
//        rustbench <dir> --mem
use std::time::{Duration, Instant};

use webp_rust::{decode, encode_lossless, encode_lossy, ImageBuffer};

const LOSSY_QUALITY: usize = 90;
const OURS_FAST_OPT: usize = 0;
const OURS_SLOW_OPT: usize = 9;
const OURS_LL_OPT: usize = 6;

const MODE_NAMES: [&str; 3] = ["lossless", "lossy-fast", "lossy-slow"];

fn main() {
    let args: Vec<String> = std::env::args().collect();
    if let Some(i) = args.iter().position(|a| a == "--mem-one") {
        measure_rss(&args[i + 1], &args[i + 2]);
        return;
    }
    let positional: Vec<&String> = args[1..].iter().filter(|a| !a.starts_with("--")).collect();
    let dir = positional
        .first()
        .map(|s| s.as_str())
        .unwrap_or("testdata/photos");
    let budget = Duration::from_millis(
        positional
            .get(1)
            .and_then(|s| s.parse().ok())
            .unwrap_or(2000),
    );
    let min_iters: u32 = positional.get(2).and_then(|s| s.parse().ok()).unwrap_or(2);
    let max_iters: u32 = positional
        .get(3)
        .and_then(|s| s.parse().ok())
        .unwrap_or(500);
    let mem_pass = args.iter().any(|a| a == "--mem");

    let mut paths: Vec<_> = std::fs::read_dir(dir)
        .unwrap_or_else(|e| panic!("read dir {dir}: {e}"))
        .filter_map(|e| e.ok().map(|e| e.path()))
        .filter(|p| {
            matches!(
                p.extension().and_then(|x| x.to_str()),
                Some("jpg") | Some("jpeg") | Some("png")
            )
        })
        .collect();
    paths.sort();

    if mem_pass {
        let self_exe = std::env::current_exe().expect("current_exe");
        for path in &paths {
            for mode in MODE_NAMES {
                let status = std::process::Command::new(&self_exe)
                    .arg("--mem-one")
                    .arg(mode)
                    .arg(path)
                    .status();
                match status {
                    Ok(s) if s.success() => {}
                    other => eprintln!("webp-rust/{mode} {}: {other:?}", path.display()),
                }
            }
        }
        return;
    }

    for path in paths {
        let name = path.file_name().unwrap().to_string_lossy().to_string();
        let img = match image::open(&path) {
            Ok(i) => i.to_rgba8(),
            Err(e) => {
                eprintln!("skip {name}: {e}");
                continue;
            }
        };
        let (w, h) = (img.width() as usize, img.height() as usize);
        let buffer = ImageBuffer {
            width: w,
            height: h,
            rgba: img.into_raw(),
        };

        let modes: Vec<(&str, Box<dyn Fn() -> Result<Vec<u8>, _>>)> = vec![
            (
                "lossless",
                Box::new(|| encode_lossless(&buffer, OURS_LL_OPT, None)),
            ),
            (
                "lossy-fast",
                Box::new(|| encode_lossy(&buffer, OURS_FAST_OPT, LOSSY_QUALITY, None)),
            ),
            (
                "lossy-slow",
                Box::new(|| encode_lossy(&buffer, OURS_SLOW_OPT, LOSSY_QUALITY, None)),
            ),
        ];

        for (mode, f) in modes {
            match measure(&f, budget, min_iters, max_iters) {
                Ok((out, iters, per_op_ms)) => {
                    let size = out.len();
                    let quality = if mode == "lossless" {
                        "-".to_string()
                    } else {
                        match psnr_of(&buffer.rgba, &out) {
                            Some(db) => format!("{db:.2}"),
                            None => {
                                eprintln!("webp-rust/{mode} {name}: psnr failed");
                                continue;
                            }
                        }
                    };
                    println!(
                        "webp-rust,{mode},{name},{w},{h},{size},{quality},{iters},{per_op_ms:.3}"
                    );
                }
                Err(e) => eprintln!("webp-rust/{mode} {name}: {e:?}"),
            }
        }
    }
}

/// Encodes one image once and reports this process's peak RSS: the source bitmap
/// and the runtime are included, since an application encoding an image pays for
/// those too.
fn measure_rss(mode: &str, path: &str) {
    let name = std::path::Path::new(path)
        .file_name()
        .unwrap()
        .to_string_lossy()
        .to_string();
    let img = match image::open(path) {
        Ok(i) => i.to_rgba8(),
        Err(e) => {
            eprintln!("skip {name}: {e}");
            std::process::exit(1);
        }
    };
    let (w, h) = (img.width() as usize, img.height() as usize);
    let buffer = ImageBuffer {
        width: w,
        height: h,
        rgba: img.into_raw(),
    };
    let encoded = match mode {
        "lossless" => encode_lossless(&buffer, OURS_LL_OPT, None),
        "lossy-fast" => encode_lossy(&buffer, OURS_FAST_OPT, LOSSY_QUALITY, None),
        "lossy-slow" => encode_lossy(&buffer, OURS_SLOW_OPT, LOSSY_QUALITY, None),
        other => {
            eprintln!("no such mode {other}");
            std::process::exit(1);
        }
    };
    if let Err(e) = encoded {
        eprintln!("webp-rust/{mode} {name}: {e:?}");
        std::process::exit(1);
    }
    let mib = peak_rss_bytes() as f64 / (1 << 20) as f64;
    let mp = (w * h) as f64 / 1e6;
    println!(
        "webp-rust,{mode},{name},{w},{h},{mp:.2},{mib:.1},{:.1}",
        mib / mp
    );
}

fn peak_rss_bytes() -> u64 {
    let mut usage: libc::rusage = unsafe { std::mem::zeroed() };
    if unsafe { libc::getrusage(libc::RUSAGE_SELF, &mut usage) } != 0 {
        return 0;
    }
    // Linux reports ru_maxrss in kilobytes, macOS in bytes.
    let scale = if cfg!(target_os = "macos") { 1 } else { 1024 };
    usage.ru_maxrss as u64 * scale
}

fn measure<E>(
    f: &dyn Fn() -> Result<Vec<u8>, E>,
    budget: Duration,
    min_iters: u32,
    max_iters: u32,
) -> Result<(Vec<u8>, u32, f64), E> {
    let out = f()?; // warmup
    let start = Instant::now();
    let mut iters: u32 = 0;
    loop {
        f()?;
        iters += 1;
        let elapsed = start.elapsed();
        if iters >= max_iters || (iters >= min_iters && elapsed >= budget) {
            let per_op_ms = elapsed.as_secs_f64() * 1000.0 / iters as f64;
            return Ok((out, iters, per_op_ms));
        }
    }
}

/// Scores encoded output against the source pixels the encoder was given, over
/// RGB only. Decoding uses webp-rust's own decoder.
fn psnr_of(src: &[u8], encoded: &[u8]) -> Option<f64> {
    let dec = decode(encoded).ok()?;
    if dec.rgba.len() != src.len() {
        return None;
    }
    let mut sum = 0.0f64;
    let mut n = 0u64;
    for (i, (&a, &b)) in src.iter().zip(dec.rgba.iter()).enumerate() {
        if i % 4 == 3 {
            continue;
        }
        let d = a as f64 - b as f64;
        sum += d * d;
        n += 1;
    }
    if sum == 0.0 {
        return Some(f64::INFINITY);
    }
    Some(10.0 * (255.0 * 255.0 / (sum / n as f64)).log10())
}
