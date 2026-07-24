// rustbench encodes images with the webp-rust library (the Rust original this Go
// port is based on) across three modes, emitting CSV lines compatible with the
// Go webpbench tool:
//
//   engine,mode,file,width,height,bytes,iters,ms_per_op
//
// Usage: rustbench <dir> [budget_ms] [min_iters] [max_iters]
use std::time::{Duration, Instant};

use webp_rust::{encode_lossless, encode_lossy, ImageBuffer};

const LOSSY_QUALITY: usize = 90;
const OURS_FAST_OPT: usize = 0;
const OURS_SLOW_OPT: usize = 9;
const OURS_LL_OPT: usize = 6;

fn main() {
    let args: Vec<String> = std::env::args().collect();
    let dir = args.get(1).map(String::as_str).unwrap_or("testdata/photos");
    let budget = Duration::from_millis(args.get(2).and_then(|s| s.parse().ok()).unwrap_or(2000));
    let min_iters: u32 = args.get(3).and_then(|s| s.parse().ok()).unwrap_or(2);
    let max_iters: u32 = args.get(4).and_then(|s| s.parse().ok()).unwrap_or(500);

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
                Ok((size, iters, per_op_ms)) => println!(
                    "webp-rust,{mode},{name},{w},{h},{size},{iters},{per_op_ms:.3}"
                ),
                Err(e) => eprintln!("webp-rust/{mode} {name}: {e:?}"),
            }
        }
    }
}

fn measure<E>(
    f: &dyn Fn() -> Result<Vec<u8>, E>,
    budget: Duration,
    min_iters: u32,
    max_iters: u32,
) -> Result<(usize, u32, f64), E> {
    let size = f()?.len(); // warmup
    let start = Instant::now();
    let mut iters: u32 = 0;
    loop {
        f()?;
        iters += 1;
        let elapsed = start.elapsed();
        if iters >= max_iters || (iters >= min_iters && elapsed >= budget) {
            let per_op_ms = elapsed.as_secs_f64() * 1000.0 / iters as f64;
            return Ok((size, iters, per_op_ms));
        }
    }
}
