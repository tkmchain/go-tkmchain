use alloc::string::String;
use core::marker::PhantomData;

use itertools::Itertools;
use p3_field::{reduce_32, Field, PrimeField, PrimeField32};

use crate::hasher::CryptographicHasher;
use crate::permutation::CryptographicPermutation;

/// Overwrite-mode sponge with injective 10-padding for variable-length input.
///
/// `PaddingFreeSponge` remains available for protocols whose input length is
/// fixed by construction (Merkle leaves and trace rows).  Callers that hash
/// attacker-controlled or otherwise variable-length data must use this type.
#[derive(Copy, Clone, Debug)]
pub struct Pad10Sponge<P, const WIDTH: usize, const RATE: usize, const OUT: usize> {
    permutation: P,
}

impl<P, const WIDTH: usize, const RATE: usize, const OUT: usize>
    Pad10Sponge<P, WIDTH, RATE, OUT>
{
    pub const fn new(permutation: P) -> Self {
        Self { permutation }
    }
}

impl<T, P, const WIDTH: usize, const RATE: usize, const OUT: usize>
    CryptographicHasher<T, [T; OUT]> for Pad10Sponge<P, WIDTH, RATE, OUT>
where
    T: Field + Copy,
    P: CryptographicPermutation<[T; WIDTH]>,
{
    fn hash_iter<I>(&self, input: I) -> [T; OUT]
    where
        I: IntoIterator<Item = T>,
    {
        assert!(RATE > 0 && RATE < WIDTH && OUT <= WIDTH);
        let mut state = [T::ZERO; WIDTH];
        let mut input = input.into_iter();
        loop {
            let mut consumed = 0;
            while consumed < RATE {
                match input.next() {
                    Some(value) => {
                        state[consumed] = value;
                        consumed += 1;
                    }
                    None => {
                        // The marker position encodes the exact short-block
                        // length.  Zeroing the remainder removes stale state
                        // from an earlier permutation.
                        state[consumed] = T::ONE;
                        for value in &mut state[(consumed + 1)..RATE] {
                            *value = T::ZERO;
                        }
                        self.permutation.permute_mut(&mut state);
                        return state[..OUT].try_into().unwrap();
                    }
                }
            }

            // A full block uses the capacity as the second 10-padding case.
            // This cannot collide with a short block, whose marker is in the
            // rate portion.
            state[RATE] = T::ONE;
            for value in &mut state[(RATE + 1)..WIDTH] {
                *value = T::ZERO;
            }
            self.permutation.permute_mut(&mut state);
        }
    }
}

/// A padding-free, overwrite-mode sponge function.
///
/// `WIDTH` is the sponge's rate plus the sponge's capacity.
#[derive(Copy, Clone, Debug)]
pub struct PaddingFreeSponge<P, const WIDTH: usize, const RATE: usize, const OUT: usize> {
    permutation: P,
}

impl<P, const WIDTH: usize, const RATE: usize, const OUT: usize>
    PaddingFreeSponge<P, WIDTH, RATE, OUT>
{
    pub const fn new(permutation: P) -> Self {
        Self { permutation }
    }
}

impl<T, P, const WIDTH: usize, const RATE: usize, const OUT: usize> CryptographicHasher<T, [T; OUT]>
    for PaddingFreeSponge<P, WIDTH, RATE, OUT>
where
    T: Default + Copy,
    P: CryptographicPermutation<[T; WIDTH]>,
{
    fn hash_iter<I>(&self, input: I) -> [T; OUT]
    where
        I: IntoIterator<Item = T>,
    {
        // static_assert(RATE < WIDTH)
        let mut state = [T::default(); WIDTH];
        let mut input = input.into_iter();

        // Itertools' chunks() is more convenient, but seems to add more overhead,
        // hence the more manual loop.
        'outer: loop {
            for i in 0..RATE {
                if let Some(x) = input.next() {
                    state[i] = x;
                } else {
                    if i != 0 {
                        self.permutation.permute_mut(&mut state);
                    }
                    break 'outer;
                }
            }
            self.permutation.permute_mut(&mut state);
        }

        state[..OUT].try_into().unwrap()
    }
}

/// A padding-free, overwrite-mode sponge function that operates natively over PF but accepts elements
/// of F: PrimeField32.
///
/// `WIDTH` is the sponge's rate plus the sponge's capacity.
#[derive(Clone, Debug)]
pub struct MultiField32PaddingFreeSponge<
    F,
    PF,
    P,
    const WIDTH: usize,
    const RATE: usize,
    const OUT: usize,
> {
    permutation: P,
    num_f_elms: usize,
    _phantom: PhantomData<(F, PF)>,
}

impl<F, PF, P, const WIDTH: usize, const RATE: usize, const OUT: usize>
    MultiField32PaddingFreeSponge<F, PF, P, WIDTH, RATE, OUT>
where
    F: PrimeField32,
    PF: Field,
{
    pub fn new(permutation: P) -> Result<Self, String> {
        if F::order() >= PF::order() {
            return Err(String::from("F::order() must be less than PF::order()"));
        }

        // Preserve every bit of the native field when decomposing it into
        // 32-bit transcript elements.  Flooring this ratio discards the
        // high limb of wide fields such as Bn254.
        let num_f_elms = PF::bits().div_ceil(F::bits());
        Ok(Self {
            permutation,
            num_f_elms,
            _phantom: PhantomData,
        })
    }
}

impl<F, PF, P, const WIDTH: usize, const RATE: usize, const OUT: usize>
    CryptographicHasher<F, [PF; OUT]> for MultiField32PaddingFreeSponge<F, PF, P, WIDTH, RATE, OUT>
where
    F: PrimeField32,
    PF: PrimeField + Default + Copy,
    P: CryptographicPermutation<[PF; WIDTH]>,
{
    fn hash_iter<I>(&self, input: I) -> [PF; OUT]
    where
        I: IntoIterator<Item = F>,
    {
        let mut state = [PF::default(); WIDTH];
        for block_chunk in &input.into_iter().chunks(RATE) {
            for (chunk_id, chunk) in (&block_chunk.chunks(self.num_f_elms))
                .into_iter()
                .enumerate()
            {
                state[chunk_id] = reduce_32(&chunk.collect_vec());
            }
            state = self.permutation.permute(state);
        }

        state[..OUT].try_into().unwrap()
    }
}

#[cfg(test)]
mod security_tests {
    use super::*;
    use p3_koala_bear::KoalaBear;
    use p3_field::FieldAlgebra;
    use crate::{CryptographicHasher, CryptographicPermutation, Permutation};

    #[derive(Copy, Clone, Debug)]
    struct IdentityPermutation;

    impl Permutation<[KoalaBear; 4]> for IdentityPermutation {
        fn permute_mut(&self, _input: &mut [KoalaBear; 4]) {}
    }

    impl CryptographicPermutation<[KoalaBear; 4]> for IdentityPermutation {}

    #[test]
    fn pad10_distinguishes_variable_lengths() {
        let sponge = Pad10Sponge::<IdentityPermutation, 4, 2, 1>::new(IdentityPermutation);
        let first = sponge.hash_iter([KoalaBear::from_canonical_u32(9)]);
        let second = sponge.hash_iter([
            KoalaBear::from_canonical_u32(9),
            KoalaBear::ZERO,
        ]);
        assert_ne!(first, second);
    }
}
