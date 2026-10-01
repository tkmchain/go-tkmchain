//! Browser entry point for the canonical Shield3 relation.
//!
//! The browser wallet uses this function from a dedicated worker. The input
//! format is byte-for-byte the same bounded request accepted by the native
//! `tkm_shield3_call` ABI; no alternate proof relation is introduced.

const BUFFER_CAPACITY: usize = 8 + super::PUBLIC_WORDS_V4 * 8 + 4 + super::MAX_PROOF_WORDS * 8;

// Fixed buffers keep the browser ABI simple and bounded. The worker clears
// both buffers after every call; no proof or witness is retained between
// requests.
static mut INPUT: [u8; BUFFER_CAPACITY] = [0; BUFFER_CAPACITY];
static mut OUTPUT: [u8; BUFFER_CAPACITY] = [0; BUFFER_CAPACITY];

#[unsafe(no_mangle)]
pub extern "C" fn tkm_shield3_wasm_input_ptr() -> usize {
    core::ptr::addr_of_mut!(INPUT) as *mut u8 as usize
}

#[unsafe(no_mangle)]
pub extern "C" fn tkm_shield3_wasm_output_ptr() -> usize {
    core::ptr::addr_of_mut!(OUTPUT) as *mut u8 as usize
}

#[unsafe(no_mangle)]
pub extern "C" fn tkm_shield3_wasm_buffer_capacity() -> usize {
    BUFFER_CAPACITY
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn tkm_shield3_wasm_run(
    operation: u32,
    length: usize,
) -> i32 {
    if length > BUFFER_CAPACITY {
        return 1;
    }
    let input = unsafe { core::slice::from_raw_parts(core::ptr::addr_of!(INPUT) as *const u8, length) };
    let result = match super::ffi::process(operation, input) {
        Ok(value) if value.len() <= BUFFER_CAPACITY => value,
        _ => return 1,
    };
    unsafe {
        core::ptr::copy_nonoverlapping(
            result.as_ptr(),
            core::ptr::addr_of_mut!(OUTPUT) as *mut u8,
            result.len(),
        );
        core::ptr::write_bytes(core::ptr::addr_of_mut!(INPUT) as *mut u8, 0, length);
        let written = result.len();
        // Store the length in the first eight bytes of the input buffer after
        // clearing the request. This avoids a second exported pointer-sized
        // global and is read by the worker before the next call.
        let length_bytes = (written as u64).to_le_bytes();
        core::ptr::copy_nonoverlapping(
            length_bytes.as_ptr(),
            core::ptr::addr_of_mut!(INPUT) as *mut u8,
            length_bytes.len(),
        );
    }
    0
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn tkm_shield3_wasm_output_length() -> usize {
    unsafe { u64::from_le_bytes(INPUT[..8].try_into().unwrap()) as usize }
}

#[unsafe(no_mangle)]
pub unsafe extern "C" fn tkm_shield3_wasm_clear() {
    unsafe {
        core::ptr::write_bytes(core::ptr::addr_of_mut!(INPUT) as *mut u8, 0, BUFFER_CAPACITY);
        core::ptr::write_bytes(core::ptr::addr_of_mut!(OUTPUT) as *mut u8, 0, BUFFER_CAPACITY);
    }
}
